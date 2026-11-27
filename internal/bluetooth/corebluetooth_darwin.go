//go:build darwin && !ios

package bluetooth

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
)

type coreBluetoothBackend struct {
	helper string
}

// PlatformBackend is the name of the compiled Bluetooth backend.
const PlatformBackend = "corebluetooth"

func newPlatformBluetooth(helper string) (Scanner, error) {
	if helper == "" {
		return nil, errors.New("run ./scripts/run-macos to build and start the CoreBluetooth helper")
	}
	return coreBluetoothBackend{helper: helper}, nil
}

func (b coreBluetoothBackend) Scan(ctx context.Context, _ ScanOptions, ready func(), observe func(ManufacturerData)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.helper)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	lines := bufio.NewScanner(stdout)
	for lines.Scan() {
		line := lines.Text()
		if line == "ready" {
			ready()
			log.Print("listening for BLE advertisements via CoreBluetooth")
			continue
		}
		if data, err := hex.DecodeString(line); err == nil && len(data) >= 2 {
			// CoreBluetooth prefixes manufacturer data with the company ID.
			observe(ManufacturerData{CompanyID: binary.LittleEndian.Uint16(data[:2]), Payload: data[2:]})
		}
	}
	if err := lines.Err(); err != nil {
		cancel()
		cmd.Wait()
		return fmt.Errorf("read CoreBluetooth helper: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("CoreBluetooth helper: %w", err)
	}
	return errors.New("CoreBluetooth helper stopped")
}
