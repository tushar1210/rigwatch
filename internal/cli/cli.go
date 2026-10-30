// Package cli wires the rigwatch commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/tushar1210/rigwatch/internal/agent"
	"github.com/tushar1210/rigwatch/internal/alert"
	"github.com/tushar1210/rigwatch/internal/collect"
	"github.com/tushar1210/rigwatch/internal/config"
	"github.com/tushar1210/rigwatch/internal/power"
	"github.com/tushar1210/rigwatch/internal/tui"
)

// Version is overwritten by goreleaser via ldflags.
var Version = "0.1.0"

type options struct {
	configPath string
	interval   time.Duration
	asJSON     bool
	demo       bool
	exitAfter  time.Duration
	tempAmber  float64
	tempRed    float64
	cpuWarn    float64
	memWarn    float64
	gpuWarn    float64
	bell       bool
	alertCmd   string
	lhmURL     string
	listen     string
	token      string
	stdio      bool
	remoteBin  string
	profile    string
	staleAfter time.Duration
}

// New returns the root command.
func New() *cobra.Command {
	opt := &options{}
	root := &cobra.Command{
		Use:   "rigwatch",
		Short: "Live machine vitals in the terminal",
		Long: `rigwatch shows CPU, memory, GPU, fans, temperatures, disks, network,
and top processes. It runs locally, as an agent, or as a client over
WebSocket or an SSH stdio tunnel.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := opt.resolve(cmd)
			if err != nil {
				return err
			}
			if opt.asJSON {
				return printJSON(cmd.Context(), collector(cfg, opt.demo))
			}
			return runTUI(cmd.Context(), cfg, opt, nil)
		},
	}
	root.PersistentFlags().StringVar(&opt.configPath, "config", "", "config file (default: $RIGWATCH_CONFIG or the user config dir)")
	root.PersistentFlags().StringVar(&opt.profile, "profile", "", "named layout of panels and thresholds")
	root.PersistentFlags().DurationVar(&opt.interval, "interval", time.Second, "sample interval")
	root.PersistentFlags().DurationVar(&opt.staleAfter, "stale-after", 0, "mark a host stale after this long without a sample")
	root.PersistentFlags().Float64Var(&opt.tempAmber, "temp-amber", 0, "amber temperature threshold °C")
	root.PersistentFlags().Float64Var(&opt.tempRed, "temp-red", 0, "red temperature threshold °C")
	root.PersistentFlags().Float64Var(&opt.cpuWarn, "cpu-warn", 0, "CPU percent alert")
	root.PersistentFlags().Float64Var(&opt.memWarn, "mem-warn", 0, "memory percent alert")
	root.PersistentFlags().Float64Var(&opt.gpuWarn, "gpu-warn", 0, "GPU utilisation alert")
	root.PersistentFlags().BoolVar(&opt.bell, "bell", false, "ring the terminal bell on a new alert")
	root.PersistentFlags().StringVar(&opt.alertCmd, "alert-command", "", "shell command to run on a new alert")
	root.PersistentFlags().StringVar(&opt.lhmURL, "lhm-url", "", "LibreHardwareMonitor data.json URL")
	root.PersistentFlags().StringVar(&opt.token, "token", "", "shared bearer token for the agent")
	root.Flags().BoolVar(&opt.asJSON, "json", false, "print one snapshot and exit")
	root.Flags().BoolVar(&opt.demo, "demo", false, "show synthetic vitals")
	root.Flags().DurationVar(&opt.exitAfter, "exit-after", 0, "quit the TUI after this duration")

	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Publish metrics over HTTP, WebSocket, SSE, or stdio",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := opt.resolve(cmd)
			if err != nil {
				return err
			}
			srv := &agent.Server{
				Token:     cfg.Token,
				Collector: collector(cfg, opt.demo),
				Interval:  cfg.Interval,
				Alerts:    alert.NewEngine(cfg),
				Command:   cfg.Alerts.Command,
			}
			if opt.stdio {
				return srv.Run(cmd.Context(), "", true, os.Stdout)
			}
			fmt.Fprintf(os.Stderr, "rigwatch agent listening on %s\n", cfg.Listen)
			return srv.Run(cmd.Context(), cfg.Listen, false, nil)
		},
	}
	agentCmd.Flags().StringVar(&opt.listen, "listen", "", "listen address")
	agentCmd.Flags().BoolVar(&opt.stdio, "stdio", false, "write NDJSON snapshots to stdout")
	agentCmd.Flags().BoolVar(&opt.demo, "demo", false, "publish synthetic vitals")

	connectCmd := &cobra.Command{
		Use:   "connect <target>...",
		Short: "Watch one or more remote agents",
		Long: `Targets are host:port, ws://host:port/ws, or ssh://user@host.

ssh:// runs "rigwatch agent --stdio" on the remote host, so no port is opened.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := opt.resolve(cmd)
			if err != nil {
				return err
			}
			var targets []agent.Target
			for _, arg := range args {
				t, err := agent.ParseTarget(arg)
				if err != nil {
					return err
				}
				targets = append(targets, t)
			}
			return runTUI(cmd.Context(), cfg, opt, targets)
		},
	}
	connectCmd.Flags().StringVar(&opt.remoteBin, "remote-bin", "rigwatch", "rigwatch binary on the SSH host")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), Version)
		},
	}
	root.AddCommand(agentCmd, connectCmd, versionCmd)
	return root
}

func (o *options) resolve(cmd *cobra.Command) (config.Config, error) {
	cfg, err := config.Load(o.configPath)
	if err != nil {
		return cfg, err
	}
	if changed(cmd, "profile") {
		cfg, err = cfg.WithProfile(o.profile)
		if err != nil {
			return cfg, err
		}
	}
	if changed(cmd, "interval") {
		cfg.Interval = o.interval
	}
	if changed(cmd, "temp-amber") {
		cfg.Threshold.TempAmber = o.tempAmber
		cfg.Threshold.CPUTempAmber = 0
		cfg.Threshold.GPUTempAmber = 0
	}
	if changed(cmd, "temp-red") {
		cfg.Threshold.TempRed = o.tempRed
		cfg.Threshold.CPUTempRed = 0
		cfg.Threshold.GPUTempRed = 0
	}
	if changed(cmd, "cpu-warn") {
		cfg.Threshold.CPUWarn = o.cpuWarn
	}
	if changed(cmd, "mem-warn") {
		cfg.Threshold.MemWarn = o.memWarn
	}
	if changed(cmd, "gpu-warn") {
		cfg.Threshold.GPUUtilWarn = o.gpuWarn
	}
	if changed(cmd, "bell") {
		cfg.Alerts.Bell = o.bell
	}
	if changed(cmd, "alert-command") {
		cfg.Alerts.Command = o.alertCmd
	}
	if changed(cmd, "lhm-url") {
		cfg.LHMURL = o.lhmURL
	}
	if changed(cmd, "token") {
		cfg.Token = o.token
	}
	if changed(cmd, "listen") && o.listen != "" {
		cfg.Listen = o.listen
	}
	if changed(cmd, "stale-after") {
		cfg.StaleAfter = o.staleAfter
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func changed(cmd *cobra.Command, name string) bool {
	if cmd.Flags().Changed(name) {
		return true
	}
	if cmd.InheritedFlags().Changed(name) {
		return true
	}
	return false
}

func collector(cfg config.Config, demo bool) collect.Collector {
	if demo {
		return collect.NewDemo()
	}
	return collect.NewLive(cfg.LHMURL)
}

func printJSON(ctx context.Context, col collect.Collector) error {
	snap, err := col.Sample(ctx)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}

func runTUI(ctx context.Context, cfg config.Config, opt *options, targets []agent.Target) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan tea.Msg, 32)
	var sources []tui.Source
	ctls := make([]*hostCtl, 1)
	if len(targets) == 0 {
		name, _ := os.Hostname()
		if name == "" {
			name = "local"
		}
		sources = []tui.Source{{Name: name, Kind: "local"}}
		ctls[0] = &hostCtl{}
		if opt.demo {
			ctls[0].set(func(context.Context) error { return errors.New("demo") })
		} else {
			ctls[0].set(func(ctx context.Context) error { return power.HaltContext(ctx) })
		}
		go pumpLocal(ctx, 0, collector(cfg, opt.demo), cfg.Interval, events)
	} else {
		sources = make([]tui.Source, len(targets))
		ctls = make([]*hostCtl, len(targets))
		for i, target := range targets {
			sources[i] = tui.Source{Name: target.Display, Kind: target.Kind}
			ctls[i] = &hostCtl{}
			go pumpRemote(ctx, i, target, cfg.Token, opt.remoteBin, cfg.EffectiveStaleAfter(), events, ctls[i])
		}
	}
	model := tui.New(tui.Options{
		Config:    cfg,
		Sources:   sources,
		Demo:      opt.demo && len(targets) == 0,
		ExitAfter: opt.exitAfter,
		Shutdown: func(index int) error {
			if index < 0 || index >= len(ctls) || ctls[index] == nil {
				return errors.New("not connected")
			}
			shutCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			return ctls[index].call(shutCtx)
		},
	})
	// Forward samples into the program.
	forward := make(chan tea.Msg, 32)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-events:
				select {
				case <-ctx.Done():
					return
				case forward <- msg:
				}
			}
		}
	}()
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-forward:
				if !ok {
					return
				}
				p.Send(msg)
			}
		}
	}()
	_, err := p.Run()
	return err
}

func pumpLocal(ctx context.Context, index int, col collect.Collector, interval time.Duration, events chan<- tea.Msg) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		snap, err := col.Sample(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			events <- tui.StatusMsg{Index: index, Err: err.Error()}
		} else {
			events <- tui.SampleMsg{Index: index, Snap: snap}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

const (
	backoffMin = time.Second
	backoffMax = 30 * time.Second
)

func pumpRemote(ctx context.Context, index int, target agent.Target, token, remoteBin string, staleAfter time.Duration, events chan<- tea.Msg, ctl *hostCtl) {
	if staleAfter < time.Second {
		staleAfter = 3 * time.Second
	}
	backoff := backoffMin
	for {
		if ctx.Err() != nil {
			return
		}
		stream, err := agent.Open(ctx, target, token, remoteBin)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			events <- tui.StatusMsg{Index: index, Reconnecting: true, Err: shortErr(err)}
			if !sleep(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		ctl.set(stream.Shutdown)
		events <- tui.StatusMsg{Index: index, Err: ""}
		got := false
		for {
			rctx, cancel := context.WithTimeout(ctx, staleAfter)
			snap, err := stream.Recv(rctx)
			cancel()
			if err != nil {
				var cmdErr *agent.CommandError
				if errors.As(err, &cmdErr) {
					events <- tui.StatusMsg{Index: index, Err: cmdErr.Error()}
					continue
				}
				ctl.set(nil)
				_ = stream.Close()
				if ctx.Err() != nil {
					return
				}
				msg := "reconnecting"
				if !errors.Is(err, context.DeadlineExceeded) {
					msg = shortErr(err)
				}
				events <- tui.StatusMsg{Index: index, Reconnecting: true, Err: msg}
				break
			}
			got = true
			backoff = backoffMin
			events <- tui.SampleMsg{Index: index, Snap: snap}
		}
		ctl.set(nil)
		if !sleep(ctx, backoff) {
			return
		}
		if !got {
			backoff = nextBackoff(backoff)
		}
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d < backoffMin {
		return backoffMin
	}
	if d > backoffMax {
		return backoffMax
	}
	return d
}

func shortErr(err error) string {
	if err == nil {
		return "disconnected"
	}
	s := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:79]) + "…"
	}
	return s
}

// hostCtl is the live power-off hook for one source.
// It is nil until that host is connected.
type hostCtl struct {
	mu sync.Mutex
	fn func(context.Context) error
}

func (c *hostCtl) set(fn func(context.Context) error) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.fn = fn
	c.mu.Unlock()
}

func (c *hostCtl) call(ctx context.Context) error {
	if c == nil {
		return errors.New("not connected")
	}
	c.mu.Lock()
	fn := c.fn
	c.mu.Unlock()
	if fn == nil {
		return errors.New("not connected")
	}
	return fn(ctx)
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
