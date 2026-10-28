# rigwatch

`rigwatch` shows live machine vitals in the terminal: CPU, memory, GPU, fans, temperatures, disks, network, and the busiest processes. It runs on the machine you are sitting at, including over SSH, and it can watch other machines.

Missing sensors are shown as unavailable with a short hint. They do not crash the UI.

## Install

```bash
go install github.com/tushar1210/rigwatch/cmd/rigwatch@latest
```

Release builds cover `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, and `windows/arm64`. See `.goreleaser.yaml`.

## Usage

Watch this machine:

```bash
rigwatch
rigwatch --interval 2s
rigwatch --json
rigwatch --demo
```

Publish samples (WebSocket at `/ws`, SSE at `/api/stream`, one shot at `/api/snapshot`):

```bash
rigwatch agent --listen :7777
rigwatch agent --listen :7777 --token "$RIGWATCH_TOKEN"
```

Watch one or more agents. `tab` moves between hosts.

```bash
rigwatch connect forge:7777 atlas:7777 --token "$RIGWATCH_TOKEN"
rigwatch connect ssh://tushar@windows-box
```

`ssh://` does not open a port. The client runs `rigwatch agent --stdio` on the remote host over OpenSSH, using your normal keys and `~/.ssh/config`. The remote binary must be on `PATH` for a non-interactive session, or pass `--remote-bin`.

```bash
rigwatch connect ssh://tushar@windows-box --remote-bin 'C:\Users\tushar\bin\rigwatch.exe'
```

## Keys

| Key | Action |
| --- | --- |
| `q`, `ctrl+c` | Quit |
| `esc` | Close the overlay, or quit |
| `tab` / `shift+tab` | Next / previous host |
| `p` | Processes |
| `c` / `m` | Sort processes by CPU or memory |
| `?` | Help |

No mouse. The view follows the terminal size. Colours use the terminal's profile and drop to plain text when colour is disabled (`NO_COLOR`, `TERM=dumb`).

## Configuration

Flags override a YAML file. The file is `--config`, then `$RIGWATCH_CONFIG`, then `~/.config/rigwatch/config.yaml` when that file exists. See `rigwatch.example.yaml`.

```yaml
interval: 1s
listen: ":7777"
token: ""
lhm_url: "http://127.0.0.1:8085/data.json"
thresholds:
  temp_amber: 75
  temp_red: 85
  cpu_warn: 90
  mem_warn: 90
  gpu_util_warn: 95
alerts:
  bell: true
  command: "notify-send rigwatch \"$RIGWATCH_KEY $RIGWATCH_VALUE\""
```

Amber and red thresholds colour temperatures in the UI. CPU, memory, and GPU utilisation use their own warn levels. A new breach can ring the terminal bell or run a command. The command sees `RIGWATCH_HOST`, `RIGWATCH_KEY`, `RIGWATCH_LEVEL`, `RIGWATCH_VALUE`, and `RIGWATCH_LIMIT`. Repeat samples of the same breach do not fire again until the reading recovers.

## Sensor support

| Reading | Linux | macOS | Windows |
| --- | --- | --- | --- |
| CPU usage, per core | gopsutil | gopsutil | gopsutil |
| CPU frequency | `cpufreq` `scaling_cur_freq`, else gopsutil | gopsutil | gopsutil |
| Load average | yes | yes | gopsutil approximation from the processor queue |
| Memory, cache, swap | yes | yes | yes; swap is labelled pagefile |
| Temperatures | `/sys/class/hwmon`, thermal zones | SMC via IOKit, no sudo | ACPI `MSAcpi_ThermalZoneTemperature` (admin), or LibreHardwareMonitor |
| Fans | hwmon tachometers | SMC `F*Ac` keys | LibreHardwareMonitor |
| NVIDIA GPU | go-nvml when built with cgo; pure-Go NVML loader when `CGO_ENABLED=0`; `nvidia-smi` if both fail | `nvidia-smi` if an eGPU driver is installed | `nvml.dll` loaded at runtime, then `nvidia-smi`, then LibreHardwareMonitor |
| AMD GPU | DRM sysfs: busy %, VRAM, hwmon temp, fan, power, clocks | — | adapter name from WMI; temps, fan, power, and clocks from LibreHardwareMonitor |
| Intel GPU | DRM sysfs, best effort (`gpu_busy_percent`, `gt_cur_freq_mhz`, hwmon) | — | same as AMD: WMI name, LibreHardwareMonitor for the rest |
| Apple GPU | — | `ioreg` `IOAccelerator` utilisation and wired unified memory; temperature from SMC | — |
| Disks and network | gopsutil, per-mount and per-interface rates | gopsutil | gopsutil |

Loopback and common virtual interfaces (`veth`, `docker`, `br-`, `cni`, and similar) are hidden. Virtual filesystems are skipped.

NVIDIA's `go-nvml` module is Linux-only and uses cgo to `dlopen` `libnvidia-ml.so.1`. The library is not linked in, so a cgo build still starts on a machine with no driver and falls back to `nvidia-smi`. Release binaries are built with `CGO_ENABLED=0`. On Linux and Windows those binaries load NVML themselves (`libnvidia-ml.so.1` or `nvml.dll`) and use `nvidia-smi` when the library is absent. macOS does not use NVML.

## Windows

CPU, memory, disks, network, and processes work without extra software.

Thermal zones from `root\wmi` `MSAcpi_ThermalZoneTemperature` need an elevated process. A normal terminal shows `run as admin` instead of a temperature.

CPU package temperatures, per-core temperatures, fan RPM, and AMD or Intel GPU telemetry are not available from stock Windows APIs. Run [LibreHardwareMonitor](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor) and enable one of:

- the WMI provider (`root\LibreHardwareMonitor`)
- the remote web server, which serves `http://127.0.0.1:8085/data.json` (override with `--lhm-url`)

`rigwatch` tries WMI first, then the JSON endpoint, then ACPI. If LibreHardwareMonitor is not running, the card says `start LibreHardwareMonitor`.

NVIDIA cards are also read from `nvml.dll` and `nvidia-smi.exe` (including the default path under `C:\Program Files\NVIDIA Corporation\NVSMI` when it is not on `PATH`).

For OpenSSH, install `rigwatch.exe` somewhere the non-interactive session can see, then:

```bash
rigwatch connect ssh://tushar@windows-box
```

## Development

```bash
gofmt -w .
gofmt -l .
go vet ./...
golangci-lint run
go test ./...
```

Cross-compiles use `CGO_ENABLED=0` for the six release targets. A normal `go build` on Linux uses cgo and go-nvml.

## License

MIT. See `LICENSE`.
