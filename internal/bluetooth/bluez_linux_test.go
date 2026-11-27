//go:build linux && !android

package bluetooth

import (
	"bytes"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestBlueZAdvertisement(t *testing.T) {
	path := dbus.ObjectPath("/org/bluez/hci0")
	payload := []byte{1, 2, 3}
	const companyID = 0x1234
	data := dbus.MakeVariant(map[uint16]dbus.Variant{companyID: dbus.MakeVariant(payload)})
	event := &dbus.Signal{
		Name: properties + ".PropertiesChanged",
		Path: "/org/bluez/hci1/dev_00_11_22_33_44_55",
		Body: []any{device, map[string]dbus.Variant{"ManufacturerData": data}, []string{}},
	}
	var received []ManufacturerData
	observe := func(data ManufacturerData) { received = append(received, data) }
	if err := handleBlueZSignal(path, event, observe); err != nil || len(received) != 0 {
		t.Fatalf("unexpected result from another adapter: %+v, %v", received, err)
	}
	event.Path = "/org/bluez/hci0/dev_00_11_22_33_44_55"
	if err := handleBlueZSignal(path, event, observe); err != nil || len(received) != 1 ||
		received[0].CompanyID != companyID || !bytes.Equal(received[0].Payload, payload) {
		t.Fatalf("expected company %04x and payload %x, got %+v, %v", companyID, payload, received, err)
	}
}
