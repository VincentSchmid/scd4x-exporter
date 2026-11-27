//go:build linux && !android

package bluetooth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	// PlatformBackend is the name of the compiled Bluetooth backend.
	PlatformBackend = "bluez"
	bluez           = "org.bluez"
	adapter         = bluez + ".Adapter1"
	device          = bluez + ".Device1"
	objects         = "org.freedesktop.DBus.ObjectManager"
	properties      = "org.freedesktop.DBus.Properties"
	bus             = "org.freedesktop.DBus"
)

type blueZBackend struct{}

func newPlatformBluetooth(_ string) (Scanner, error) {
	return blueZBackend{}, nil
}

func (blueZBackend) Scan(ctx context.Context, options ScanOptions, ready func(), observe func(ManufacturerData)) error {
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
	filter := map[string]dbus.Variant{
		"Transport":     dbus.MakeVariant("le"),
		"DuplicateData": dbus.MakeVariant(true),
	}
	if options.NamePrefix != "" {
		filter["Pattern"] = dbus.MakeVariant(options.NamePrefix)
	}
	if err := hci.CallWithContext(setup, adapter+".SetDiscoveryFilter", 0, filter).Err; err != nil {
		return err
	}
	if err := hci.CallWithContext(setup, adapter+".StartDiscovery", 0).Err; err != nil {
		return err
	}
	ready()
	log.Printf("listening for BLE advertisements on %s", path)
	for {
		select {
		case <-conn.Context().Done():
			return errors.New("D-Bus disconnected")
		case event, open := <-signals:
			if !open {
				return errors.New("D-Bus signal stream closed")
			}
			if err := handleBlueZSignal(path, event, observe); err != nil {
				return err
			}
		}
	}
}

func handleBlueZSignal(path dbus.ObjectPath, event *dbus.Signal, observe func(ManufacturerData)) error {
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
			observeBlueZData(interfaces[device]["ManufacturerData"], observe)
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
			observeBlueZData(changed["ManufacturerData"], observe)
		}
	}
	return nil
}

func observeBlueZData(manufacturerData dbus.Variant, observe func(ManufacturerData)) {
	data, _ := manufacturerData.Value().(map[uint16]dbus.Variant)
	for id, value := range data {
		if payload, ok := value.Value().([]byte); ok {
			observe(ManufacturerData{CompanyID: id, Payload: payload})
		}
	}
}
