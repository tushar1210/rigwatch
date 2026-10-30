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
| `c` / `m` / `g` | Sort processes by CPU, memory, or GPU |
| `a` | Alert history |
| `?` | Help |

## Config

Flags override YAML. Lookup order is `--config`, then `$RIGWATCH_CONFIG`, then `~/.config/rigwatch/config.yaml` when that file exists.

Copy [`rigwatch.example.yaml`](rigwatch.example.yaml). Thresholds color the panels. Alerts use those limits, and a rolling baseline.

```yaml
thresholds:
  temp_amber: 75
  temp_red: 85
  cpu_warn: 90
  mem_warn: 90
  gpu_util_warn: 95
  disk_warn: 90
  fan_pct_warn: 95
  fan_rpm_warn: 0
alerts:
  enabled: true
  bell: true
  command: "notify-send rigwatch \"$RIGWATCH_KEY $RIGWATCH_VALUE\""
  cooldown: 2m
  metrics: [cpu, memory, gpu, temp, disk, fan]
  baseline:
    enabled: true
    window: 10m
    sustain: 1m
    margin_percent: 20
    margin_temp: 10
    margin_rpm: 400
    min_samples: 12
```

CPU and GPU temperatures follow `temp_amber` / `temp_red` unless `cpu_temp_*` or `gpu_temp_*` is set. `fan_rpm_warn: 0` leaves RPM off. `metrics` limits which signals are watched.

The baseline is the mean of CPU, memory, GPU, temperature, and fan samples inside `window`, excluding the current point. Disks use the fixed percent only. It alerts when a reading stays above that mean by the margin (`margin_percent`, `margin_temp`, or `margin_rpm`) for `sustain`. `min_samples` is how many points the mean needs first.

The same alert does not repeat until the reading recovers and `cooldown` has passed (default 2m). Amber rising to red still notifies. The banner clears on recovery. `cooldown: 0` re-arms as soon as the reading recovers.

The TUI draws a one-line banner, a status count, and highlights the panel. It rings the bell (BEL) and runs `command` when those are set. The agent writes an `alert` event on stdio, SSE (`event: alert`), and WebSocket, and runs `command` too. Snapshot frames are unchanged. WebSocket samples stay bare JSON. An alert frame is `{"type":"alert","alert":{...}}`.

The command sees `RIGWATCH_HOST`, `RIGWATCH_KEY`, `RIGWATCH_LEVEL`, `RIGWATCH_KIND`, `RIGWATCH_METRIC`, `RIGWATCH_VALUE`, and `RIGWATCH_LIMIT`. The TUI and the agent each run it, so set the command on the side that should notify.

`a` lists the last alerts for this session: time, host, metric, level. `alerts.history` is how many to keep (default 32, `0` disables it). The list stays in memory. It is not written to disk.

## Trends

CPU, GPU, memory, and temperature panels draw a one-line sparkline, labeled `cpu`, `gpu`, `mem`, and `temp`. The line is the recent samples on this screen. A gap is a missing reading.

## Stale hosts

If no sample arrives for `stale_after` (default three intervals, at least 3s), the header says `stale` and the panels lose their live border. A dropped SSH or agent link is marked stale immediately and shows `reconnect`. The client dials again on its own, waiting 1s, then 2s, up to 30s. `--stale-after` overrides the config key. On a remote agent, set it above that agent's sample interval.

## Profiles

`profile` or `--profile` loads a named layout. The profile chooses which panels appear and overlays thresholds on the top-level ones. Flags still win. Omit the name to show every panel.

```bash
rigwatch --profile gaming
```

```yaml
profile: gaming
profiles:
  gaming:
    panels: [cpu, gpu, mem, temp]
    thresholds:
      temp_red: 95
      gpu_util_warn: 99
  render:
    panels: [cpu, gpu, mem, temp, disk, fan]
    thresholds:
      temp_amber: 70
      temp_red: 82
```

Panel names are `cpu`, `gpu`, `mem`, `temp`, `fan`, `disk`, and `net`.

## Processes

`p` opens the process list. `c`, `m`, and `g` sort by CPU, memory, and GPU. When `nvidia-smi` is installed, GPU% comes from `pmon` and VRAM from compute-app memory. Both columns stay hidden when that data is missing, including machines without an NVIDIA driver. The lookup runs beside the sample so the one-second `pmon` delay does not freeze the UI. Apple and AMD GPUs do not fill these columns.

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
