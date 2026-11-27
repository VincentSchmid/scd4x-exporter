# SCD4x BLE exporter

A small Go exporter for the Sensirion MyCO2 (SCD4x CO2 Gadget). It reads
live Bluetooth LE advertisements via Linux BlueZ without pairing or connecting
to the gadget. It has no database, sidecar, or Prometheus client dependency.

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

CI tests each PR and main push. Each green main push publishes a Linux arm64
and amd64 image to `ghcr.io/vincentschmid/scd4x-exporter`, tagged `0.1.<CI run>`
and `sha-<full commit>`. Pin deployments by version and manifest digest.

The daily Renovate workflow uses the repository secret `RENOVATE_TOKEN`. Use
a repository-scoped credential that can write contents, workflows, pull
requests, issues and commit statuses and read required checks. Do not use
`GITHUB_TOKEN`: Renovate PRs and their merges need to trigger CI. GitHub
auto-merge and the required `test` check gate eligible dependency PRs; major
updates are left for review.
