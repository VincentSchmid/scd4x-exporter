package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/VincentSchmid/scd4x-exporter/internal/bluetooth"
	"github.com/VincentSchmid/scd4x-exporter/internal/sensirion"
)

const freshFor = 90 * time.Second

type sample struct {
	sensirion.Reading
	seen time.Time
}

type exporter struct {
	mu      sync.Mutex
	ready   bool
	samples map[string]sample
}

func (e *exporter) observe(reading sensirion.Reading, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, previous := range e.samples {
		if now.Sub(previous.seen) >= freshFor {
			delete(e.samples, id)
		}
	}
	e.samples[reading.GadgetID] = sample{Reading: reading, seen: now}
}

func (e *exporter) setReady(ready bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ready = ready
	if !ready {
		clear(e.samples)
	}
}

func (e *exporter) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		e.mu.Lock()
		ready := e.ready
		e.mu.Unlock()
		if !ready {
			http.Error(w, "Bluetooth scan unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, e.metrics())
	})
	return mux
}

func (e *exporter) metrics() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	var output strings.Builder
	for id, reading := range e.samples {
		if now.Sub(reading.seen) >= freshFor {
			continue
		}
		fmt.Fprintf(&output, "scd4x_co2_ppm{gadget=%q} %d\n", id, reading.CO2PPM)
		fmt.Fprintf(&output, "scd4x_temperature_celsius{gadget=%q} %.2f\n", id, reading.TemperatureCelsius)
		fmt.Fprintf(&output, "scd4x_relative_humidity_percent{gadget=%q} %.2f\n", id, reading.RelativeHumidityPercent)
		fmt.Fprintf(&output, "scd4x_last_seen_timestamp_seconds{gadget=%q} %d\n", id, reading.seen.Unix())
	}
	return output.String()
}

func (e *exporter) runScanner(ctx context.Context, scanner bluetooth.Scanner) {
	options := bluetooth.ScanOptions{NamePrefix: sensirion.DiscoveryNamePrefix}
	for ctx.Err() == nil {
		err := scanner.Scan(ctx, options, func() { e.setReady(true) }, func(data bluetooth.ManufacturerData) {
			if reading, ok := sensirion.Decode(data.CompanyID, data.Payload); ok {
				e.observe(reading, time.Now())
			}
		})
		e.setReady(false)
		if ctx.Err() != nil {
			return
		}
		log.Printf("Bluetooth scan unavailable: %v; retrying in 15s", err)
		select {
		case <-ctx.Done():
		case <-time.After(15 * time.Second):
		}
	}
}

// Run serves metrics and scans for readings until ctx is canceled or serving fails.
func Run(ctx context.Context, addr string, scanner bluetooth.Scanner) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	e := &exporter{samples: make(map[string]sample)}
	srv := &http.Server{
		Addr:              addr,
		Handler:           e.httpHandler(),
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	stopClosing := context.AfterFunc(ctx, func() { srv.Close() })
	defer stopClosing()

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		e.runScanner(ctx, scanner)
	}()
	err := srv.ListenAndServe()
	cancel()
	<-scanDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
