package bluetooth

import (
	"context"
	"fmt"
	"runtime"
)

// Scanner receives live Bluetooth LE advertisements.
type Scanner interface {
	// Scan calls ready once discovery starts, then delivers manufacturer data
	// until discovery stops or the context is canceled.
	Scan(ctx context.Context, options ScanOptions, ready func(), observe func(ManufacturerData)) error
}

type ScanOptions struct {
	// NamePrefix is a discovery hint on backends that support name filtering.
	NamePrefix string
}

type ManufacturerData struct {
	CompanyID uint16
	Payload   []byte // Excludes the company ID.
}

// New returns the compiled backend. Use "auto" or the platform's backend name.
// The helper path is required on macOS.
func New(name, helper string) (Scanner, error) {
	if name != "auto" && name != PlatformBackend {
		return nil, fmt.Errorf("Bluetooth backend %q is not available on %s (compiled backend: %s)", name, runtime.GOOS, PlatformBackend)
	}
	return newPlatformBluetooth(helper)
}
