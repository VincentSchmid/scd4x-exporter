//go:build (!linux && !darwin) || android || ios

package bluetooth

import (
	"fmt"
	"runtime"
)

// PlatformBackend indicates there is no Bluetooth backend for this OS.
const PlatformBackend = "none"

func newPlatformBluetooth(_ string) (Scanner, error) {
	return nil, fmt.Errorf("no Bluetooth backend for %s", runtime.GOOS)
}
