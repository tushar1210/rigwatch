# Changelog

## 0.1.0 - 2026-10-28

### Added

- Local terminal UI for CPU, memory, GPU, fans, temperatures, disks, network, and top processes.
- `rigwatch --json` for a single snapshot.
- `rigwatch agent` with a JSON snapshot, server-sent events, and a WebSocket stream. Optional bearer token.
- `rigwatch connect` for one or more `host:port` targets, plus `ssh://user@host` which runs the agent over SSH stdio.
- Linux hwmon and thermal-zone sensors.
- macOS SMC temperatures and fans through IOKit without sudo, and Apple GPU utilisation from `ioreg`.
- Windows ACPI thermal zones, LibreHardwareMonitor WMI, and the LibreHardwareMonitor JSON endpoint.
- NVIDIA via go-nvml when cgo is enabled, a pure-Go NVML loader when it is not, and `nvidia-smi` as a fallback.
- AMD and Intel GPU readings from sysfs on Linux.
- Configurable temperature colours (amber 75 °C, red 85 °C) and optional bell or command on a new breach.
- GitHub Actions CI, goreleaser builds, and unit tests with fake collectors.
