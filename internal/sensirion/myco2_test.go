package sensirion

import (
	"math"
	"testing"
)

func TestDecodeOnlyLiveMyCO2(t *testing.T) {
	payload := []byte{0, 8, 0x12, 0x34, 0x66, 0x66, 0x00, 0x80, 0x20, 0x03}
	wrongFrame := append([]byte(nil), payload...)
	wrongFrame[0] = 1
	wrongFormat := append([]byte(nil), payload...)
	wrongFormat[1] = 6
	for _, tt := range []struct {
		name    string
		id      uint16
		payload []byte
	}{
		{"wrong company", 0x1234, payload},
		{"short payload", 0x06d5, payload[:2]},
		{"wrong frame", 0x06d5, wrongFrame},
		{"wrong format", 0x06d5, wrongFormat},
	} {
		if r, ok := Decode(tt.id, tt.payload); ok {
			t.Errorf("%s: unexpectedly decoded %+v", tt.name, r)
		}
	}
	r, ok := Decode(0x06d5, payload)
	if !ok {
		t.Fatal("valid MyCO2 payload was rejected")
	}
	if r.GadgetID != "1234" || r.CO2PPM != 800 ||
		math.Abs(r.TemperatureCelsius-25) > 0.01 || math.Abs(r.RelativeHumidityPercent-50) > 0.01 {
		t.Fatalf("unexpected reading: %+v", r)
	}
}
