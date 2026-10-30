package tui

import (
	"math"
	"os"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tushar1210/rigwatch/internal/alert"
	"github.com/tushar1210/rigwatch/internal/config"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

// Source is one local or remote machine in the pager.
type Source struct {
	Name string
	Kind string
}

// SampleMsg delivers a snapshot for one source.
type SampleMsg struct {
	Index int
	Snap  metrics.Snapshot
}

// StatusMsg reports a connection problem. An empty Err means the link is up.
// Reconnecting is set while the client waits to dial again.
type StatusMsg struct {
	Index        int
	Err          string
	Reconnecting bool
}

// Options configures the TUI.
type Options struct {
	Config    config.Config
	Sources   []Source
	Demo      bool
	ExitAfter time.Duration
}

// Model is the Bubble Tea model.
type Model struct {
	cfg        config.Config
	sources    []sourceState
	active     int
	width      int
	height     int
	keys       keyMap
	help       help.Model
	table      table.Model
	showHelp   bool
	showProc   bool
	showAlerts bool
	sortBy     int
	demo       bool
	exitAfter  time.Duration
	staleAfter time.Duration
	alerts     *alert.Engine
	history    []alert.Event
	ready      bool
}

const (
	sortByCPU = iota
	sortByMem
	sortByGPU
)

type sourceState struct {
	name         string
	kind         string
	snap         metrics.Snapshot
	err          string
	seen         bool
	stale        bool
	reconnecting bool
	sampledAt    time.Time
	hist         history
}

type history struct {
	cpu  []float64
	mem  []float64
	gpu  []float64
	temp []float64
}

type exitNow struct{}

// New builds a model. Sources must contain at least one entry.
func New(opts Options) Model {
	cfg := opts.Config
	if cfg.Interval == 0 {
		cfg = config.Default()
	}
	sources := opts.Sources
	if len(sources) == 0 {
		sources = []Source{{Name: "local", Kind: "local"}}
	}
	states := make([]sourceState, len(sources))
	for i, src := range sources {
		name := src.Name
		if name == "" {
			name = "host"
		}
		states[i] = sourceState{name: name, kind: src.Kind}
	}
	h := help.New()
	h.Styles.ShortKey = styleDim()
	h.Styles.ShortDesc = styleFaint()
	h.Styles.ShortSeparator = styleFaint()
	h.Styles.FullKey = styleAccent()
	h.Styles.FullDesc = styleText()
	h.Styles.FullSeparator = styleFaint()
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "PID", Width: 8},
			{Title: "CPU", Width: 8},
			{Title: "MEM", Width: 8},
			{Title: "RSS", Width: 10},
			{Title: "NAME", Width: 24},
		}),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	st := table.DefaultStyles()
	st.Header = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDim)).Bold(true).Padding(0, 1)
	st.Cell = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Padding(0, 1)
	st.Selected = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Bold(true).Padding(0, 1)
	tbl.SetStyles(st)
	return Model{
		cfg:        cfg,
		sources:    states,
		keys:       newKeys(),
		help:       h,
		table:      tbl,
		demo:       opts.Demo,
		exitAfter:  opts.ExitAfter,
		staleAfter: cfg.EffectiveStaleAfter(),
		alerts:     alert.NewEngine(cfg),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.scheduleTick()}
	if m.exitAfter > 0 {
		cmds = append(cmds, tea.Tick(m.exitAfter, func(time.Time) tea.Msg { return exitNow{} }))
	}
	return tea.Batch(cmds...)
}

func (m Model) scheduleTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg{now: t} })
}

type tickMsg struct {
	now time.Time
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.ready = true
		m.syncTable()
		return m, nil
	case tea.KeyMsg:
		return m.onKey(msg)
	case SampleMsg:
		return m.onSample(msg)
	case StatusMsg:
		return m.onStatus(msg), nil
	case tickMsg:
		m.markStale(msg.now)
		return m, m.scheduleTick()
	case exitNow:
		return m, tea.Quit
	}
	if m.showProc {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Back):
		if m.showHelp || m.showProc || m.showAlerts {
			m.showHelp = false
			m.showProc = false
			m.showAlerts = false
			m.help.ShowAll = false
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.help.ShowAll = m.showHelp
		if m.showHelp {
			m.showProc = false
			m.showAlerts = false
		}
		return m, nil
	case key.Matches(msg, m.keys.Alerts):
		m.showAlerts = !m.showAlerts
		if m.showAlerts {
			m.showHelp = false
			m.showProc = false
			m.help.ShowAll = false
		}
		return m, nil
	case key.Matches(msg, m.keys.Proc):
		m.showProc = !m.showProc
		if m.showProc {
			m.showHelp = false
			m.showAlerts = false
			m.help.ShowAll = false
			m.syncTable()
		}
		return m, nil
	case key.Matches(msg, m.keys.Next):
		if len(m.sources) > 1 {
			m.active = (m.active + 1) % len(m.sources)
			m.syncTable()
		}
		return m, nil
	case key.Matches(msg, m.keys.Prev):
		if len(m.sources) > 1 {
			m.active = (m.active - 1 + len(m.sources)) % len(m.sources)
			m.syncTable()
		}
		return m, nil
	case key.Matches(msg, m.keys.SortCPU):
		m.sortBy = sortByCPU
		m.syncTable()
		return m, nil
	case key.Matches(msg, m.keys.SortMem):
		m.sortBy = sortByMem
		m.syncTable()
		return m, nil
	case key.Matches(msg, m.keys.SortGPU):
		m.sortBy = sortByGPU
		m.syncTable()
		return m, nil
	}
	if m.showProc {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onSample(msg SampleMsg) (tea.Model, tea.Cmd) {
	if msg.Index < 0 || msg.Index >= len(m.sources) {
		return m, nil
	}
	src := &m.sources[msg.Index]
	src.snap = msg.Snap
	src.err = ""
	src.seen = true
	src.stale = false
	src.reconnecting = false
	src.sampledAt = time.Now()
	if msg.Snap.Host.Hostname != "" {
		src.name = msg.Snap.Host.Hostname
	}
	temp := sampleTemp(msg.Snap.Temps)
	gpu := math.NaN()
	if len(msg.Snap.GPUs) > 0 && msg.Snap.GPUs[0].Util.Available {
		gpu = msg.Snap.GPUs[0].Util.Value
	}
	seed := m.demo && len(src.hist.cpu) == 0
	if seed {
		src.hist.cpu = seedWave(msg.Snap.CPU.UsagePercent, 48)
		src.hist.mem = seedWave(msg.Snap.Memory.UsedPercent, 48)
		if !math.IsNaN(gpu) {
			src.hist.gpu = seedWave(gpu, 48)
		}
		if !math.IsNaN(temp) {
			src.hist.temp = seedWave(temp, 48)
		}
	}
	src.hist.cpu = appendCap(src.hist.cpu, msg.Snap.CPU.UsagePercent, 60)
	src.hist.mem = appendCap(src.hist.mem, msg.Snap.Memory.UsedPercent, 60)
	src.hist.gpu = appendCap(src.hist.gpu, gpu, 60)
	src.hist.temp = appendCap(src.hist.temp, temp, 60)
	if m.showProc && msg.Index == m.active {
		m.syncTable()
	}
	host := src.name
	evs := m.alerts.Evaluate(host, msg.Snap)
	var cmds []tea.Cmd
	if len(evs) > 0 && m.cfg.Alerts.Bell {
		cmds = append(cmds, ringBell())
	}
	m.remember(evs)
	for _, ev := range evs {
		if m.cfg.Alerts.Command != "" {
			alert.RunCommand(m.cfg.Alerts.Command, ev)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) onStatus(msg StatusMsg) Model {
	if msg.Index < 0 || msg.Index >= len(m.sources) {
		return m
	}
	src := &m.sources[msg.Index]
	src.err = msg.Err
	src.reconnecting = msg.Reconnecting
	if src.seen && (msg.Err != "" || msg.Reconnecting) {
		src.stale = true
	}
	return m
}

func (m *Model) markStale(now time.Time) {
	after := m.staleAfter
	if after <= 0 {
		after = 3 * time.Second
	}
	for i := range m.sources {
		src := &m.sources[i]
		if !src.seen || src.sampledAt.IsZero() || src.stale {
			continue
		}
		if now.Sub(src.sampledAt) > after {
			src.stale = true
		}
	}
}

func (m *Model) remember(evs []alert.Event) {
	n := m.cfg.Alerts.History
	if n <= 0 || len(evs) == 0 {
		return
	}
	for _, ev := range evs {
		if ev.At.IsZero() {
			ev.At = time.Now()
		}
		m.history = append(m.history, ev)
	}
	if len(m.history) > n {
		m.history = append([]alert.Event(nil), m.history[len(m.history)-n:]...)
	}
}

func sampleTemp(temps []metrics.Temp) float64 {
	if t, ok := metrics.HeadlineTemp(temps); ok {
		return t.Celsius
	}
	best := math.NaN()
	for _, t := range temps {
		if math.IsNaN(best) || t.Celsius > best {
			best = t.Celsius
		}
	}
	return best
}

func (m *Model) syncTable() {
	snap := m.current().snap
	rows := append([]metrics.Process(nil), snap.Processes...)
	sort.Slice(rows, func(i, j int) bool {
		switch m.sortBy {
		case sortByMem:
			return rows[i].MemPercent > rows[j].MemPercent
		case sortByGPU:
			return gpuSortKey(rows[i]) > gpuSortKey(rows[j])
		default:
			return rows[i].CPUPercent > rows[j].CPUPercent
		}
	})
	showGPU, showVRAM := false, false
	for _, p := range rows {
		if p.HasGPU {
			showGPU = true
		}
		if p.HasGPUMem {
			showVRAM = true
		}
	}
	width := m.width
	if width < 40 {
		width = 40
	}
	fixed := 8 + 8 + 8 + 10
	if showGPU {
		fixed += 8
	}
	if showVRAM {
		fixed += 10
	}
	nameW := width - fixed - 6
	if nameW < 12 {
		nameW = 12
	}
	cols := []table.Column{
		{Title: "PID", Width: 8},
		{Title: "CPU", Width: 8},
		{Title: "MEM", Width: 8},
		{Title: "RSS", Width: 10},
	}
	if showGPU {
		cols = append(cols, table.Column{Title: "GPU", Width: 8})
	}
	if showVRAM {
		cols = append(cols, table.Column{Title: "VRAM", Width: 10})
	}
	cols = append(cols, table.Column{Title: "NAME", Width: nameW})
	m.table.SetColumns(cols)
	body := make([]table.Row, 0, len(rows))
	for _, p := range rows {
		row := table.Row{
			itoa(int(p.PID)),
			metrics.FormatPercent(p.CPUPercent),
			metrics.FormatPercent(p.MemPercent),
			metrics.FormatBytes(p.RSS),
		}
		if showGPU {
			row = append(row, gpuCell(p))
		}
		if showVRAM {
			row = append(row, vramCell(p))
		}
		row = append(row, p.Name)
		body = append(body, row)
	}
	m.table.SetRows(body)
	h := m.height - 6
	if h < 5 {
		h = 5
	}
	m.table.SetHeight(h)
	m.table.SetWidth(width)
}

func (m Model) current() sourceState {
	if len(m.sources) == 0 {
		return sourceState{name: "local"}
	}
	if m.active < 0 || m.active >= len(m.sources) {
		return m.sources[0]
	}
	return m.sources[m.active]
}

func ringBell() tea.Cmd {
	return func() tea.Msg {
		f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			_, _ = os.Stdout.Write([]byte{7})
			return nil
		}
		_, _ = f.Write([]byte{7})
		_ = f.Close()
		return nil
	}
}

func gpuSortKey(p metrics.Process) float64 {
	if !p.HasGPU {
		return -1
	}
	return p.GPUPercent
}

func gpuCell(p metrics.Process) string {
	if !p.HasGPU {
		return "-"
	}
	return metrics.FormatPercent(p.GPUPercent)
}

func vramCell(p metrics.Process) string {
	if !p.HasGPUMem {
		return "-"
	}
	return metrics.FormatBytes(p.GPUMemBytes)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
