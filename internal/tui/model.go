package tui

import (
	"fmt"
	"math"
	"os"
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
	// Kill overrides process termination. sig is sigTerm (15) or sigKill (9).
	Kill func(pid int32, sig int) error
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
	sortBy     metrics.ProcessSort
	procRows   []metrics.Process
	kill       func(pid int32, sig int) error
	killAsk    *killAsk
	procNote   string
	demo       bool
	exitAfter  time.Duration
	staleAfter time.Duration
	alerts     *alert.Engine
	history    []alert.Event
	ready      bool
}

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

type tickMsg struct {
	now time.Time
}

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
	kill := opts.Kill
	if kill == nil {
		kill = killPID
	}
	return Model{
		cfg:        cfg,
		sources:    states,
		keys:       newKeys(),
		help:       h,
		table:      tbl,
		kill:       kill,
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

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.ready = true
		m.syncTable(true)
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
	if m.killAsk != nil && m.showProc {
		return m.onKillKey(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Back):
		if m.showHelp || m.showProc || m.showAlerts {
			m.closeProc()
			m.showHelp = false
			m.showAlerts = false
			m.help.ShowAll = false
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.help.ShowAll = m.showHelp
		if m.showHelp {
			m.closeProc()
			m.showAlerts = false
		}
		return m, nil
	case key.Matches(msg, m.keys.Alerts):
		m.showAlerts = !m.showAlerts
		if m.showAlerts {
			m.closeProc()
			m.showHelp = false
			m.help.ShowAll = false
		}
		return m, nil
	case key.Matches(msg, m.keys.Proc):
		if m.showProc {
			m.closeProc()
			return m, nil
		}
		m.showProc = true
		m.showHelp = false
		m.showAlerts = false
		m.help.ShowAll = false
		m.syncTable(true)
		return m, nil
	case key.Matches(msg, m.keys.Next):
		if len(m.sources) > 1 {
			m.active = (m.active + 1) % len(m.sources)
			m.syncTable(false)
		}
		return m, nil
	case key.Matches(msg, m.keys.Prev):
		if len(m.sources) > 1 {
			m.active = (m.active - 1 + len(m.sources)) % len(m.sources)
			m.syncTable(false)
		}
		return m, nil
	case key.Matches(msg, m.keys.SortCPU):
		m.setSort(metrics.SortByCPU)
		return m, nil
	case key.Matches(msg, m.keys.SortMem):
		m.setSort(metrics.SortByRSS)
		return m, nil
	case key.Matches(msg, m.keys.SortGPU):
		m.setSort(metrics.SortByGPU)
		return m, nil
	case key.Matches(msg, m.keys.SortVRAM):
		m.setSort(metrics.SortByVRAM)
		return m, nil
	case key.Matches(msg, m.keys.Kill):
		if m.showProc {
			m.armKill()
		}
		return m, nil
	}
	if m.showProc {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) closeProc() {
	m.showProc = false
	m.killAsk = nil
	m.procNote = ""
}

func (m *Model) setSort(by metrics.ProcessSort) {
	if m.sortBy == by {
		m.syncTable(true)
		return
	}
	m.sortBy = by
	m.table.SetCursor(0)
	m.syncTable(false)
}

func (m Model) onKillKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Back), msg.String() == "n":
		m.killAsk = nil
		return m, nil
	case msg.String() == "y":
		return m.confirmKill()
	case msg.String() == "9":
		if m.killAsk != nil {
			m.killAsk.sig = sigKill
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) armKill() {
	m.procNote = ""
	if m.demo {
		m.procNote = "demo"
		return
	}
	if kind := m.current().kind; kind != "" && kind != "local" {
		m.procNote = "local only"
		return
	}
	sel, ok := m.selectedProc()
	if !ok || sel.PID <= 1 {
		m.procNote = "refused"
		return
	}
	m.killAsk = newKillAsk(sel.PID, sel.Name)
}

func (m Model) confirmKill() (tea.Model, tea.Cmd) {
	ask := m.killAsk
	m.killAsk = nil
	if ask == nil {
		return m, nil
	}
	if m.demo {
		m.procNote = "demo"
		return m, nil
	}
	if kind := m.current().kind; kind != "" && kind != "local" {
		m.procNote = "local only"
		return m, nil
	}
	sel, ok := m.selectedProc()
	if !ok || sel.PID != ask.pid {
		m.procNote = "process changed"
		return m, nil
	}
	if sel.PID <= 1 {
		m.procNote = "refused"
		return m, nil
	}
	if m.kill == nil {
		m.procNote = fmt.Sprintf("%d failed", ask.pid)
		return m, nil
	}
	if err := m.kill(ask.pid, ask.sig); err != nil {
		m.procNote = fmt.Sprintf("%d %s", ask.pid, killFailure(err))
		return m, nil
	}
	m.procNote = fmt.Sprintf("sent %s %d", sigLabel(ask.sig), ask.pid)
	return m, nil
}

func (m Model) selectedProc() (metrics.Process, bool) {
	if len(m.procRows) == 0 {
		return metrics.Process{}, false
	}
	i := m.table.Cursor()
	if i < 0 || i >= len(m.procRows) {
		return metrics.Process{}, false
	}
	return m.procRows[i], true
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
		m.syncTable(true)
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

func (m *Model) syncTable(keepPID bool) {
	var prev int32
	if keepPID {
		if sel, ok := m.selectedProc(); ok {
			prev = sel.PID
		}
	}
	rows := metrics.SortProcesses(m.current().snap.Processes, m.sortBy)
	m.procRows = rows
	body := make([]table.Row, len(rows))
	for i, p := range rows {
		body[i] = table.Row{p.Name}
	}
	m.table.SetRows(body)
	if len(rows) == 0 {
		return
	}
	idx := 0
	if prev != 0 {
		for i, p := range rows {
			if p.PID == prev {
				idx = i
				break
			}
		}
	}
	m.table.SetCursor(idx)
	h := m.height - 6
	if h < 5 {
		h = 5
	}
	width := m.width
	if width < 40 {
		width = 40
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
