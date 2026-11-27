package sensirion

import (
	"encoding/binary"
	"encoding/hex"
)

const companyID = 0x06d5

// DiscoveryNamePrefix is the MyCO2 device name prefix used as a scan hint.
const DiscoveryNamePrefix = "MyCO2"

// Reading holds the measurements from a live MyCO2 advertisement.
type Reading struct {
	GadgetID                string
	CO2PPM                  uint16
	TemperatureCelsius      float64
	RelativeHumidityPercent float64
}

// Decode parses a live MyCO2 reading. The payload excludes the company ID.
func Decode(id uint16, payload []byte) (Reading, bool) {
	// Live frame (0x00), temperature/humidity/CO2 format (0x08).
	if id != companyID || len(payload) < 10 || payload[0] != 0 || payload[1] != 8 {
		return Reading{}, false
	}
	return Reading{
		GadgetID:                hex.EncodeToString(payload[2:4]),
		CO2PPM:                  binary.LittleEndian.Uint16(payload[8:10]),
		TemperatureCelsius:      -45 + 175*float64(binary.LittleEndian.Uint16(payload[4:6]))/65535,
		RelativeHumidityPercent: 100 * float64(binary.LittleEndian.Uint16(payload[6:8])) / 65535,
	}, true
}
