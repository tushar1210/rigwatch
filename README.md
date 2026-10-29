# rigwatch

Cross-platform terminal monitor for machine vitals: CPU and GPU temperatures, fans, memory, disks, network, and the busiest processes.

It watches the machine you are on, or other hosts over SSH and WebSocket. A missing sensor stays on screen with a short hint. It does not crash the UI.

![Demo dashboard from rigwatch --demo: CPU, GPU, memory, fans, temperatures, disks, and network](docs/demo.png)

*Demo dashboard (`rigwatch --demo`) on a synthetic Ubuntu host.*

![Process list sorted by CPU](docs/processes.png)

*Top processes (`p`), still from `rigwatch --demo`.*

![Two-host pager, header shows 1/2](docs/connect.png)

*Two hosts (`rigwatch connect`). The header shows `1/2`. `tab` switches machines.*

## Install

```bash
go install github.com/tushar1210/rigwatch@latest
```

Prebuilt binaries for macOS, Linux, and Windows (`amd64` and `arm64`) are on the [v0.1.0 release](https://github.com/tushar1210/rigwatch/releases/tag/v0.1.0). SHA256 checksums are in [`checksums.txt`](https://github.com/tushar1210/rigwatch/releases/download/v0.1.0/checksums.txt).

| | amd64 | arm64 |
| --- | --- | --- |
| Linux | [tar.gz](https://github.com/tushar1210/rigwatch/releases/download/v0.1.0/rigwatch_0.1.0_linux_amd64.tar.gz) | [tar.gz](https://github.com/tushar1210/rigwatch/releases/download/v0.1.0/rigwatch_0.1.0_linux_arm64.tar.gz) |
| macOS | [tar.gz](https://github.com/tushar1210/rigwatch/releases/download/v0.1.0/rigwatch_0.1.0_darwin_amd64.tar.gz) | [tar.gz](https://github.com/tushar1210/rigwatch/releases/download/v0.1.0/rigwatch_0.1.0_darwin_arm64.tar.gz) |
| Windows | [zip](https://github.com/tushar1210/rigwatch/releases/download/v0.1.0/rigwatch_0.1.0_windows_amd64.zip) | [zip](https://github.com/tushar1210/rigwatch/releases/download/v0.1.0/rigwatch_0.1.0_windows_arm64.zip) |

## Usage

This machine:

```bash
rigwatch
rigwatch --interval 2s
rigwatch --json     # one JSON snapshot, then exit
rigwatch --demo     # synthetic vitals, no sensors required
```

Other machines. `tab` and `shift+tab` move between hosts.

```bash
rigwatch connect ssh://user@host
rigwatch connect forge:7777 atlas:7777 --token "$RIGWATCH_TOKEN"
```

`ssh://` does not open a port. The client runs `rigwatch agent --stdio` on the remote host with your usual keys and `~/.ssh/config`. The remote binary must be on `PATH` for a non-interactive session, or pass `--remote-bin`.

Publish samples for those clients (WebSocket `/ws`, SSE `/api/stream`, one shot `/api/snapshot`):

```bash
rigwatch agent --listen :7777
rigwatch agent --listen :7777 --token "$RIGWATCH_TOKEN"
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

## Config

Flags override YAML. Lookup order is `--config`, then `$RIGWATCH_CONFIG`, then `~/.config/rigwatch/config.yaml` when that file exists.

Copy [`rigwatch.example.yaml`](rigwatch.example.yaml). Thresholds color the UI and, with `alerts`, can ring the bell or run a command the first time a limit is crossed:

```yaml
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

The command sees `RIGWATCH_HOST`, `RIGWATCH_KEY`, `RIGWATCH_LEVEL`, `RIGWATCH_VALUE`, and `RIGWATCH_LIMIT`. The same breach does not fire again until the reading recovers.

## Platforms

macOS, Windows, and Linux.

| | Linux | macOS | Windows |
| --- | --- | --- | --- |
| CPU, memory, disks, network, processes | yes | yes | yes |
| Temperatures and fans | hwmon | SMC, no sudo | LibreHardwareMonitor, or ACPI zones as admin |
| NVIDIA GPU | NVML or `nvidia-smi` | `nvidia-smi` when a driver is installed | `nvml.dll` or `nvidia-smi`, no extra setup |
| AMD / Intel GPU | DRM sysfs | — | LibreHardwareMonitor |
| Apple GPU | — | `ioreg` and SMC | — |

### Windows

CPU, memory, disks, network, and processes need no extra software. NVIDIA cards are read from `nvml.dll` and `nvidia-smi.exe` the same way.

Stock Windows APIs do not expose package temperature or fan RPM. For those, run [LibreHardwareMonitor](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor) and turn on its WMI provider or the web server at `http://127.0.0.1:8085/data.json` (`--lhm-url` overrides that). If it is not running, the card says `start LibreHardwareMonitor`.

## Develop

```bash
gofmt -w .
gofmt -l .
go test ./...
```

Release binaries are `CGO_ENABLED=0` builds for the six targets in [`.goreleaser.yaml`](.goreleaser.yaml).

## License

MIT. See [LICENSE](LICENSE).
