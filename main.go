package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	bluez      = "org.bluez"
	adapter    = bluez + ".Adapter1"
	device     = bluez + ".Device1"
	objects    = "org.freedesktop.DBus.ObjectManager"
	properties = "org.freedesktop.DBus.Properties"
	bus        = "org.freedesktop.DBus"
	freshFor   = 90 * time.Second
)

type sample struct {
	co2         uint16
	temperature float64
	humidity    float64
	seen        time.Time
}

type exporter struct {
	mu      sync.Mutex
	ready   bool
	samples map[string]sample
}

func decode(manufacturerData dbus.Variant) (string, sample, bool) {
	data, ok := manufacturerData.Value().(map[uint16]dbus.Variant)
	if !ok {
		return "", sample{}, false
	}
	// Sensirion manufacturer data: live advertisement, T/RH/CO2 format.
	payload, ok := data[0x06d5].Value().([]byte)
	if !ok || len(payload) < 10 || payload[0] != 0 || payload[1] != 8 {
		return "", sample{}, false
	}
	return hex.EncodeToString(payload[2:4]), sample{
		co2:         binary.LittleEndian.Uint16(payload[8:10]),
		temperature: -45 + 175*float64(binary.LittleEndian.Uint16(payload[4:6]))/65535,
		humidity:    100 * float64(binary.LittleEndian.Uint16(payload[6:8])) / 65535,
	}, true
}

func (e *exporter) observe(data dbus.Variant, now time.Time) {
	id, reading, ok := decode(data)
	if !ok {
		return
	}
	reading.seen = now
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, previous := range e.samples {
		if now.Sub(previous.seen) >= freshFor {
			delete(e.samples, id)
		}
	}
	e.samples[id] = reading
}

func (e *exporter) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if !e.ready {
			http.Error(w, "Bluetooth scan unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		e.mu.Lock()
		defer e.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		now := time.Now()
		var output strings.Builder
		for id, reading := range e.samples {
			if now.Sub(reading.seen) >= freshFor {
				continue
			}
			fmt.Fprintf(&output, "scd4x_co2_ppm{gadget=%q} %d\n", id, reading.co2)
			fmt.Fprintf(&output, "scd4x_temperature_celsius{gadget=%q} %.2f\n", id, reading.temperature)
			fmt.Fprintf(&output, "scd4x_relative_humidity_percent{gadget=%q} %.2f\n", id, reading.humidity)
			fmt.Fprintf(&output, "scd4x_last_seen_timestamp_seconds{gadget=%q} %d\n", id, reading.seen.Unix())
		}
		fmt.Fprint(w, output.String())
	})
	return mux
}

func (e *exporter) scan(ctx context.Context) error {
	setup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)
	if err := conn.AddMatchSignalContext(setup, dbus.WithMatchSender(bus), dbus.WithMatchInterface(bus),
		dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, bluez)); err != nil {
		return err
	}
	var hardware map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := conn.Object(bluez, "/").CallWithContext(setup, objects+".GetManagedObjects", 0).Store(&hardware); err != nil {
		return err
	}
	var path dbus.ObjectPath
	for candidate, interfaces := range hardware {
		if interfaces[adapter]["Powered"].Value() == true && (path == "" || candidate < path) {
			path = candidate
		}
	}
	if path == "" {
		return errors.New("no powered BlueZ Bluetooth adapter")
	}
	for _, match := range [][]dbus.MatchOption{
		{dbus.WithMatchInterface(objects), dbus.WithMatchMember("InterfacesAdded")},
		{dbus.WithMatchInterface(objects), dbus.WithMatchMember("InterfacesRemoved")},
		{dbus.WithMatchInterface(properties), dbus.WithMatchPathNamespace(path)},
	} {
		if err := conn.AddMatchSignalContext(setup, append(match, dbus.WithMatchSender(bluez))...); err != nil {
			return err
		}
	}
	hci := conn.Object(bluez, path)
	if err := hci.CallWithContext(setup, adapter+".SetDiscoveryFilter", 0, map[string]dbus.Variant{
		"Transport":     dbus.MakeVariant("le"),
		"DuplicateData": dbus.MakeVariant(true),
		"Pattern":       dbus.MakeVariant("MyCO2"),
	}).Err; err != nil {
		return err
	}
	if err := hci.CallWithContext(setup, adapter+".StartDiscovery", 0).Err; err != nil {
		return err
	}
	e.mu.Lock()
	e.ready = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.ready = false
		clear(e.samples)
		e.mu.Unlock()
	}()
	log.Printf("listening for MyCO2 advertisements on %s", path)
	for {
		select {
		case <-conn.Context().Done():
			return errors.New("D-Bus disconnected")
		case event, open := <-signals:
			if !open {
				return errors.New("D-Bus signal stream closed")
			}
			if err := e.handleSignal(path, event); err != nil {
				return err
			}
		}
	}
}

func (e *exporter) handleSignal(path dbus.ObjectPath, event *dbus.Signal) error {
	switch event.Name {
	case bus + ".NameOwnerChanged":
		return errors.New("BlueZ restarted")
	case objects + ".InterfacesRemoved":
		if len(event.Body) == 2 && event.Body[0] == path {
			return errors.New("Bluetooth adapter removed")
		}
	case objects + ".InterfacesAdded":
		if len(event.Body) != 2 {
			return nil
		}
		devicePath, _ := event.Body[0].(dbus.ObjectPath)
		interfaces, _ := event.Body[1].(map[string]map[string]dbus.Variant)
		if strings.HasPrefix(string(devicePath), string(path)+"/") {
			e.observe(interfaces[device]["ManufacturerData"], time.Now())
		}
	case properties + ".PropertiesChanged":
		if len(event.Body) != 3 {
			return nil
		}
		changed, _ := event.Body[1].(map[string]dbus.Variant)
		if event.Body[0] == adapter && event.Path == path && (changed["Powered"].Value() == false || changed["Discovering"].Value() == false) {
			return errors.New("Bluetooth scan stopped")
		}
		if event.Body[0] == device && strings.HasPrefix(string(event.Path), string(path)+"/") {
			e.observe(changed["ManufacturerData"], time.Now())
		}
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	e := &exporter{samples: make(map[string]sample)}
	go func() {
		for ctx.Err() == nil {
			if err := e.scan(ctx); ctx.Err() == nil {
				log.Printf("Bluetooth scan unavailable: %v; retrying in 15s", err)
			}
			select {
			case <-ctx.Done():
			case <-time.After(15 * time.Second):
			}
		}
	}()
	server := &http.Server{Addr: ":9869", Handler: e.httpHandler(), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
