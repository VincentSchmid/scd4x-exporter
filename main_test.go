package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestAdvertisementToMetrics(t *testing.T) {
	e := &exporter{ready: true, samples: make(map[string]sample)}
	path := dbus.ObjectPath("/org/bluez/hci0")
	data := dbus.MakeVariant(map[uint16]dbus.Variant{
		0x06d5: dbus.MakeVariant([]byte{0, 8, 0x12, 0x34, 0x66, 0x66, 0x00, 0x80, 0x20, 0x03}),
	})
	event := &dbus.Signal{
		Name: properties + ".PropertiesChanged",
		Path: "/org/bluez/hci1/dev_00_11_22_33_44_55",
		Body: []any{device, map[string]dbus.Variant{"ManufacturerData": data}, []string{}},
	}
	if err := e.handleSignal(path, event); err != nil || len(e.samples) != 0 {
		t.Fatalf("unexpected reading from another adapter: %v", e.samples)
	}
	event.Path = "/org/bluez/hci0/dev_00_11_22_33_44_55"
	if err := e.handleSignal(path, event); err != nil {
		t.Fatal(err)
	}
	handler := e.httpHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, line := range []string{`scd4x_co2_ppm{gadget="1234"} 800`, `scd4x_temperature_celsius{gadget="1234"} 25.00`, `scd4x_relative_humidity_percent{gadget="1234"} 50.00`} {
		if !strings.Contains(response.Body.String(), line+"\n") {
			t.Errorf("missing %q in %s", line, response.Body.String())
		}
	}
	e.observe(data, time.Now().Add(-freshFor))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Body.Len() != 0 {
		t.Errorf("expired reading still exported: %s", response.Body.String())
	}
}

func TestIgnoresOtherAdvertisementFormats(t *testing.T) {
	for _, data := range []dbus.Variant{
		{},
		dbus.MakeVariant(map[uint16]dbus.Variant{0x06d5: dbus.MakeVariant([]byte{0, 8})}),
		dbus.MakeVariant(map[uint16]dbus.Variant{0x06d5: dbus.MakeVariant([]byte{0, 6, 0, 0, 0, 0, 0, 0, 0, 0})}),
	} {
		if _, _, ok := decode(data); ok {
			t.Errorf("accepted unsupported advertisement: %v", data)
		}
	}
}
