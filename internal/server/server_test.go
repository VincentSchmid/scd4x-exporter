package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VincentSchmid/scd4x-exporter/internal/sensirion"
)

func TestReadingToMetrics(t *testing.T) {
	e := &exporter{ready: true, samples: make(map[string]sample)}
	reading := sensirion.Reading{GadgetID: "1234", CO2PPM: 800, TemperatureCelsius: 25, RelativeHumidityPercent: 50}
	e.observe(reading, time.Now())
	handler := e.httpHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, line := range []string{`scd4x_co2_ppm{gadget="1234"} 800`, `scd4x_temperature_celsius{gadget="1234"} 25.00`, `scd4x_relative_humidity_percent{gadget="1234"} 50.00`} {
		if !strings.Contains(response.Body.String(), line+"\n") {
			t.Errorf("missing %q in %s", line, response.Body.String())
		}
	}
	e.observe(reading, time.Now().Add(-freshFor))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Body.Len() != 0 {
		t.Errorf("expired reading still exported: %s", response.Body.String())
	}
}
