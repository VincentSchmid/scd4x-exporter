# SCD4x BLE exporter

A small Go exporter for the Sensirion MyCO2 (SCD4x CO2 Gadget). It reads
live Bluetooth LE advertisements without pairing or connecting
to the gadget. It has no database, sidecar, or Prometheus client dependency.

Go includes one Bluetooth backend per target OS: CoreBluetooth on macOS and
BlueZ on Linux. `-bluetooth auto` (the default) uses the compiled backend, and
`-bluetooth corebluetooth` or `-bluetooth bluez` can explicitly require that
backend. Discovery retries every 15 seconds if it fails.

The backends in `internal/bluetooth` implement `bluetooth.Scanner` and deliver
raw manufacturer data. `internal/sensirion` filters and decodes MyCO2
advertisements into readings, which the exporter publishes as metrics.
`server_command.go` configures the CLI; `internal/server` runs discovery and
serves the HTTP endpoints.

Docker images target Linux, including `linux/arm64` images built on an Apple
Silicon Mac. Docker Desktop runs them in a Linux VM, so they cannot use the
Mac's CoreBluetooth stack. Run the exporter natively to use the Mac's radio.

## macOS

With Go and `swiftc` (from Xcode or the Xcode Command Line Tools) installed,
run `./scripts/run-macos`. It builds and starts the native CoreBluetooth helper
and binds the exporter to `127.0.0.1:9869`. Turn on Bluetooth and allow access
if macOS prompts; check **System Settings > Privacy & Security > Bluetooth**
if access is denied. Keep the broadcasting MyCO2 gadget nearby.

In another terminal, run `./scripts/check-metrics`. It exits successfully only
if discovery is ready and at least one gadget has all four metrics. You can
also pass a different base URL, e.g. `./scripts/check-metrics http://localhost:9870`.

## Linux

Run `go run .` on a Linux host with BlueZ, a powered Bluetooth adapter, and
permission for the current user to access BlueZ on the system D-Bus. In a
container, mount the host system D-Bus socket read-only and set
`DBUS_SYSTEM_BUS_ADDRESS` to `unix:path=` followed by its path in the container.
The image runs as UID/GID 65534. A read-only socket mount still permits sending
D-Bus messages; host D-Bus policy determines what this UID can do.

`GET /ready` returns 204 while BLE discovery is active. Prometheus scrapes
`GET /metrics` on port 9869. Gauges are keyed by the gadget ID in the packet:

- `scd4x_co2_ppm`
- `scd4x_temperature_celsius`
- `scd4x_relative_humidity_percent`
- `scd4x_last_seen_timestamp_seconds`

After 90 seconds without an advertisement, readings disappear from the scrape.
The receiver has to stay in Bluetooth range. The decoder accepts Sensirion
manufacturer ID `0x06d5`, live frame `0x00`, and MyCO2 sample format `0x08`.

## Releases and dependency updates

CI runs Linux tests on PRs, pushes to `main`, and pushes of version tags
starting with `v` followed by a digit. Successful publishing runs build a
`linux/arm64` image for the Raspberry Pi and push it to
`ghcr.io/vincentschmid/scd4x-exporter`. Main builds use `0.1.<CI run>`;
tag builds use the exact Git tag (for example, `v0.2.0`). Both also publish
`sha-<full commit>`. The workflow summary shows the image pin with its digest.
Build the macOS helper locally with `./scripts/run-macos`.

The daily Renovate workflow uses the repository secret `RENOVATE_TOKEN`. Use
a repository-scoped credential that can write contents, workflows, pull
requests, issues and commit statuses and read required checks. Do not use
`GITHUB_TOKEN`: Renovate PRs and their merges need to trigger CI. GitHub
auto-merge and the required `test` check gate eligible dependency PRs; major
updates are left for review.
