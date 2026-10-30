package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tushar1210/rigwatch/internal/alert"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

type severity int

const (
	sevOK severity = iota
	sevAmber
	sevRed
)

func (m Model) View() string {
	if m.width < 20 {
		return "rigwatch\n"
	}
	header := m.renderHeader()
	var body string
	switch {
	case m.showHelp:
		body = m.renderHelp()
	case m.showAlerts:
		body = m.renderAlerts()
	case m.showProc:
		body = m.renderProcs()
	default:
		body = m.renderDash()
	}
	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) renderHeader() string {
	src := m.current()
	snap := src.snap
	host := src.name
	if snap.Host.Hostname != "" {
		host = snap.Host.Hostname
	}
	var b strings.Builder
	b.WriteString(styleAccent().Render("rigwatch"))
	b.WriteString("  ")
	if len(m.sources) > 1 {
		b.WriteString(styleDim().Render(fmt.Sprintf("%d/%d", m.active+1, len(m.sources))))
		b.WriteString("  ")
	}
	b.WriteString(styleText().Render(host))
	if src.stale {
		b.WriteString(styleValue(colorAmber).Render("  stale"))
	}
	if src.reconnecting {
		b.WriteString(styleDim().Render("  reconnect"))
	}
	if snap.Host.OS != "" || snap.Host.Platform != "" {
		b.WriteString(styleDim().Render("  " + snap.Host.PlatformLine()))
	}
	if src.seen {
		b.WriteString(styleDim().Render("  up " + metrics.FormatUptime(snap.Host.UptimeSeconds)))
		if snap.CPU.Load1.Available {
			b.WriteString(styleDim().Render(fmt.Sprintf("  load %.2f %.2f %.2f", snap.CPU.Load1.Value, snap.CPU.Load5.Value, snap.CPU.Load15.Value)))
		}
	}
	line := ansi.Truncate(b.String(), m.width, "…")
	extra := ""
	if banner := m.alertBanner(host); banner != "" {
		extra += "\n" + banner
	}
	switch {
	case m.powerAsk:
		extra += "\n" + styleValue(colorRed).Render("SHUT DOWN "+host+"?  y  n")
	case m.powerNote != "":
		extra += "\n" + styleValue(colorAmber).Render(m.powerNote)
	}
	switch {
	case src.err != "" && src.err != "reconnecting":
		label := "down"
		if src.reconnecting {
			label = "reconnect"
		}
		if src.stale {
			extra += "\n" + styleDim().Render(ansi.Truncate(src.err, m.width, "…"))
		} else {
			extra += "\n" + styleValue(colorRed).Render(label) + styleDim().Render("  "+ansi.Truncate(src.err, max(8, m.width-12), "…"))
		}
	case !src.stale:
		if notes := metrics.DedupNotes(snap.Notes); len(notes) > 0 {
			extra += "\n" + styleDim().Render(ansi.Truncate(strings.Join(notes, " · "), m.width, "…"))
		}
	}
	rule := styleFaint().Render(strings.Repeat("─", max(1, m.width)))
	return line + extra + "\n" + rule
}

func (m Model) renderFooter() string {
	status := m.alertStatus(hostOf(m.current()))
	var keys help.KeyMap = m.keys
	if m.showProc && !m.showHelp {
		keys = procKeyMap{m.keys}
	}
	view := m.help.View(keys)
	if status == "" {
		return view
	}
	return status + "\n" + view
}

func (m Model) renderHelp() string {
	th := m.cfg.Threshold
	lines := []string{
		styleAccent().Render("keys"),
		styleDim().Render("q quit    esc back    tab host    p processes    a alerts"),
		styleDim().Render("c cpu   m mem   g gpu   G vram   x kill   ! shut down   ? close"),
		"",
		styleAccent().Render("thresholds"),
		styleDim().Render(fmt.Sprintf("temp amber %.0f°C    red %.0f°C", th.TempAmber, th.TempRed)),
		styleDim().Render(fmt.Sprintf("cpu %.0f%%    memory %.0f%%    gpu %.0f%%    disk %.0f%%", th.CPUWarn, th.MemWarn, th.GPUUtilWarn, th.DiskWarn)),
		"",
		styleDim().Render("A missing sensor stays on screen with a hint. It does not stop the view."),
	}
	if m.cfg.Alerts.Enabled {
		lines = append(lines, styleDim().Render("alerts on a crossing or baseline, then cooldown"))
		lines = append(lines, styleDim().Render("a last alerts, kept in memory for this session"))
	}
	if m.cfg.Profile != "" {
		lines = append(lines, styleDim().Render("profile "+m.cfg.Profile))
	}
	lines = append(lines, styleDim().Render("stale host reconnects on its own"))
	lines = append(lines, styleDim().Render("! then y powers off the selected host"))
	return strings.Join(lines, "\n")
}

func (m Model) renderProcs() string {
	src := m.current()
	if !src.seen {
		return styleDim().Render("waiting for a sample")
	}
	if len(src.snap.Processes) == 0 {
		return styleDim().Render("no processes")
	}
	title := styleAccent().Render("processes") + styleDim().Render("  by "+sortLabel(m.sortBy))
	var lines []string
	lines = append(lines, title)
	switch {
	case m.killAsk != nil:
		lines = append(lines, styleValue(colorAmber).Render(m.killAsk.prompt()))
	case m.procNote != "":
		lines = append(lines, styleValue(noteColor(m.procNote)).Render(m.procNote))
	}
	lines = append(lines, m.renderProcTable())
	return strings.Join(lines, "\n")
}

func sortLabel(by metrics.ProcessSort) string {
	switch by {
	case metrics.SortByRSS:
		return "memory"
	case metrics.SortByGPU:
		return "gpu"
	case metrics.SortByVRAM:
		return "vram"
	default:
		return "cpu"
	}
}

func noteColor(note string) string {
	switch {
	case strings.Contains(note, "denied") || strings.Contains(note, "failed") || note == "refused":
		return colorRed
	default:
		return colorDim
	}
}

func (m Model) renderProcTable() string {
	rows := m.procRows
	showGPU := procHasGPU(rows)
	nameW := procNameWidth(m.width, showGPU)
	var b strings.Builder
	b.WriteString(procHeader(showGPU, nameW))
	start, end := m.procWindow()
	for i := start; i < end; i++ {
		b.WriteByte('\n')
		b.WriteString(m.procLine(i, rows[i], showGPU, nameW))
	}
	return b.String()
}

func (m Model) procWindow() (start, end int) {
	n := len(m.procRows)
	limit := m.height - 8
	if limit < 4 {
		limit = 4
	}
	if n <= limit {
		return 0, n
	}
	cur := m.table.Cursor()
	if cur < 0 {
		cur = 0
	}
	start = cur - limit/2
	if start < 0 {
		start = 0
	}
	end = start + limit
	if end > n {
		end = n
		start = end - limit
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func procHasGPU(rows []metrics.Process) bool {
	for _, p := range rows {
		if p.GPUUtilOK || p.GPUMemOK || p.GPUMemPctOK {
			return true
		}
	}
	return false
}

func procNameWidth(screen int, showGPU bool) int {
	used := 1 + 1 + 7 + 6 + 6 + 8
	fields := 7
	if showGPU {
		used += 6 + 8
		fields += 2
	}
	nameW := screen - used - (fields - 1)
	if nameW < 8 {
		nameW = 8
	}
	return nameW
}

func procHeader(showGPU bool, nameW int) string {
	parts := []string{
		col(" ", 1),
		col("#", 1),
		col("PID", 7),
		col("CPU", 6),
		col("MEM", 6),
		col("RSS", 8),
	}
	if showGPU {
		parts = append(parts, col("GPU", 6), col("VRAM", 8))
	}
	parts = append(parts, col("NAME", nameW))
	return styleDim().Render(strings.Join(parts, " "))
}

func (m Model) procLine(i int, p metrics.Process, showGPU bool, nameW int) string {
	selected := i == m.table.Cursor()
	st := rankStyle(i, selected)
	mark := " "
	if selected {
		mark = ">"
	}
	rank := " "
	if i < 3 {
		rank = strconv.Itoa(i + 1)
	}
	parts := []string{
		st.Render(col(mark, 1)),
		st.Render(col(rank, 1)),
		col(strconv.FormatInt(int64(p.PID), 10), 7),
		col(metrics.FormatPercent(p.CPUPercent), 6),
		col(metrics.FormatPercent(p.MemPercent), 6),
		col(metrics.FormatBytes(p.RSS), 8),
	}
	if showGPU {
		parts = append(parts, col(formatGPUUtil(p), 6), col(formatGPUMem(p), 8))
	}
	name := p.Name
	if name == "" {
		name = "?"
	}
	parts = append(parts, st.Render(col(trunc(name, nameW), nameW)))
	return ansi.Truncate(strings.Join(parts, " "), max(8, m.width), "…")
}

func rankStyle(i int, selected bool) lipgloss.Style {
	if selected || i == 0 {
		return styleAccent()
	}
	if i == 1 {
		return styleText().Bold(true)
	}
	if i == 2 {
		return styleValue(colorAmber)
	}
	return styleText()
}

func formatGPUUtil(p metrics.Process) string {
	if !p.GPUUtilOK {
		return "-"
	}
	return metrics.FormatPercent(p.GPUPercent)
}

func formatGPUMem(p metrics.Process) string {
	if p.GPUMemOK {
		return metrics.FormatBytes(p.GPUMem)
	}
	if p.GPUMemPctOK {
		return metrics.FormatPercent(p.GPUMemPercent)
	}
	return "-"
}

func col(s string, n int) string {
	w := ansi.StringWidth(s)
	if w > n {
		return ansi.Truncate(s, n, "")
	}
	return s + strings.Repeat(" ", n-w)
}

func (m Model) renderAlerts() string {
	lines := []string{styleAccent().Render("alerts")}
	if len(m.history) == 0 {
		lines = append(lines, styleDim().Render("none"))
		return strings.Join(lines, "\n")
	}
	limit := m.height - 6
	if limit < 3 {
		limit = 3
	}
	start := len(m.history) - limit
	if start < 0 {
		start = 0
	}
	for i := len(m.history) - 1; i >= start; i-- {
		ev := m.history[i]
		metric := ev.Metric
		if metric == "" {
			metric = ev.Key
		}
		host := trunc(ev.Host, 12)
		line := fmt.Sprintf("%s  %s  %s  %s", ev.At.Format("15:04:05"), pad(host, 12), pad(trunc(metric, 8), 8), ev.Level)
		color := colorAmber
		if ev.Level == "red" {
			color = colorRed
		}
		lines = append(lines, styleValue(color).Render(ansi.Truncate(line, max(8, m.width), "…")))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderDash() string {
	src := m.current()
	if !src.seen && src.err == "" {
		return styleDim().Render("sampling…")
	}
	if !src.seen {
		return styleDim().Render("no sample yet")
	}
	type panel struct {
		name string
		col  int
		draw func(int) string
	}
	w := m.width
	col := w
	wide := w >= 100
	if wide {
		col = (w - 1) / 2
	}
	all := []panel{
		{name: "cpu", col: 0, draw: func(width int) string { return m.cpuCard(width, src) }},
		{name: "mem", col: 0, draw: func(width int) string { return m.memCard(width, src) }},
		{name: "gpu", col: 1, draw: func(width int) string { return m.gpuCard(width, src) }},
		{name: "fan", col: 1, draw: func(width int) string { return m.fanCard(width, src) }},
		{name: "temp", col: 1, draw: func(width int) string { return m.tempCard(width, src) }},
		{name: "disk", col: 0, draw: func(width int) string { return m.diskCard(width, src) }},
		{name: "net", col: 0, draw: func(width int) string { return m.netCard(width, src) }},
	}
	var left, right []string
	var narrow []string
	for _, p := range all {
		if !m.showPanel(p.name) {
			continue
		}
		if !wide {
			narrow = append(narrow, p.draw(w))
			continue
		}
		if p.col == 1 {
			right = append(right, p.draw(col))
		} else {
			left = append(left, p.draw(col))
		}
	}
	if !wide {
		if len(narrow) == 0 {
			return styleDim().Render("no panels")
		}
		return lipgloss.JoinVertical(lipgloss.Left, narrow...)
	}
	switch {
	case len(left) == 0 && len(right) == 0:
		return styleDim().Render("no panels")
	case len(left) == 0:
		return lipgloss.JoinVertical(lipgloss.Left, right...)
	case len(right) == 0:
		return lipgloss.JoinVertical(lipgloss.Left, left...)
	default:
		return lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.JoinVertical(lipgloss.Left, left...),
			" ",
			lipgloss.JoinVertical(lipgloss.Left, right...),
		)
	}
}

func (m Model) showPanel(name string) bool {
	if len(m.cfg.Panels) == 0 {
		return true
	}
	for _, p := range m.cfg.Panels {
		if p == name {
			return true
		}
	}
	return false
}

func (m Model) cpuCard(width int, src sourceState) string {
	snap := src.snap
	temp, hasTemp := metrics.HeadlineTemp(snap.Temps)
	sev := sevOK
	if hasTemp {
		sev = worse(sev, m.tempSev(temp.Celsius))
	}
	if snap.CPU.UsagePercent >= m.cfg.Threshold.CPUWarn {
		sev = worse(sev, sevAmber)
	}
	sev = worse(sev, m.metricSev(hostOf(src), "cpu"))
	sev = worse(sev, m.keySev(hostOf(src), "cpu temp"))
	title := styleAccent().Render("cpu") + "  " + m.pctColored(snap.CPU.UsagePercent, m.cfg.Threshold.CPUWarn)
	if snap.CPU.FrequencyMHz.Available {
		title += styleDim().Render("  " + metrics.FormatFixed(snap.CPU.FrequencyMHz.Value, " MHz"))
	} else {
		title += styleDim().Render("  freq unavailable")
	}
	if hasTemp {
		title += "  " + m.tempText(temp.Celsius)
	} else {
		title += styleDim().Render("  temp unavailable")
	}
	inner := m.innerWidth(width)
	var lines []string
	lines = append(lines, title)
	lines = append(lines, m.trend("cpu", src.hist.cpu, inner))
	lines = append(lines, m.coreLines(snap.CPU.PerCore, inner)...)
	return m.card(width, sev, lines...)
}

func (m Model) memCard(width int, src sourceState) string {
	mem := src.snap.Memory
	sev := sevOK
	if mem.UsedPercent >= m.cfg.Threshold.MemWarn {
		sev = sevAmber
	}
	sev = worse(sev, m.metricSev(hostOf(src), "memory"))
	title := styleAccent().Render("memory") + "  " + styleText().Render(metrics.FormatBytes(mem.Used)+" / "+metrics.FormatBytes(mem.Total))
	title += "  " + m.pctColored(mem.UsedPercent, m.cfg.Threshold.MemWarn)
	inner := m.innerWidth(width)
	lines := []string{
		title,
		m.bar(mem.UsedPercent, inner, m.cfg.Threshold.MemWarn, 98),
		styleDim().Render("avail " + metrics.FormatBytes(mem.Available)),
	}
	if mem.CachedAvailable {
		lines[2] += styleDim().Render("   cache " + metrics.FormatBytes(mem.Cached))
	}
	label := mem.SwapLabel
	if label == "" {
		label = "swap"
	}
	lines = append(lines, styleDim().Render(fmt.Sprintf("%s %s / %s", label, metrics.FormatBytes(mem.SwapUsed), metrics.FormatBytes(mem.SwapTotal))))
	lines = append(lines, m.trend("mem", src.hist.mem, inner))
	return m.card(width, sev, lines...)
}

func (m Model) gpuCard(width int, src sourceState) string {
	gpus := src.snap.GPUs
	if len(gpus) == 0 {
		return m.card(width, sevOK, styleAccent().Render("gpu"), styleDim().Render("none detected"))
	}
	sev := sevOK
	var lines []string
	lines = append(lines, styleAccent().Render("gpu"))
	inner := m.innerWidth(width)
	limit := len(gpus)
	if limit > 2 {
		limit = 2
	}
	for i := 0; i < limit; i++ {
		g := gpus[i]
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, styleText().Render(trunc(g.Name, inner)))
		util := labeledGauge("util", g.Util, metrics.FormatPercent)
		temp := styleDim().Render("temp unavailable")
		if g.Temp.Available {
			temp = m.tempPlain(g.Temp.Value)
			sev = worse(sev, m.tempSev(g.Temp.Value))
		} else if g.Temp.Hint != "" {
			temp = styleDim().Render("temp unavailable · " + g.Temp.Hint)
		}
		fan := fanText(g)
		lines = append(lines, util+styleDim().Render("  ")+temp+styleDim().Render("  ")+fan)
		vram := "vram unavailable"
		if g.VRAMOK {
			if g.Unified {
				vram = "wired " + metrics.FormatBytes(g.VRAMUsed) + "  unified"
			} else if g.VRAMTotal > 0 {
				vram = "vram " + metrics.FormatBytes(g.VRAMUsed) + " / " + metrics.FormatBytes(g.VRAMTotal)
			} else {
				vram = "vram " + metrics.FormatBytes(g.VRAMUsed)
			}
		} else if g.VRAMHint != "" {
			vram = "vram unavailable · " + g.VRAMHint
		}
		power := labeledGauge("power", g.Power, func(v float64) string { return metrics.FormatFixed(v, " W") })
		lines = append(lines, styleDim().Render(vram+"   ")+power)
		core := labeledGauge("core", g.ClockCore, func(v float64) string { return metrics.FormatFixed(v, "") })
		mem := labeledGauge("mem", g.ClockMem, func(v float64) string { return metrics.FormatFixed(v, " MHz") })
		lines = append(lines, core+styleDim().Render("  ")+mem)
		if i == 0 && hasFinite(src.hist.gpu) {
			lines = append(lines, m.trend("gpu", src.hist.gpu, inner))
		}
		if g.Util.Available && g.Util.Value >= m.cfg.Threshold.GPUUtilWarn {
			sev = worse(sev, sevAmber)
		}
	}
	sev = worse(sev, m.metricSev(hostOf(src), "gpu"))
	if len(gpus) > limit {
		lines = append(lines, styleDim().Render(fmt.Sprintf("+%d more", len(gpus)-limit)))
	}
	return m.card(width, sev, lines...)
}

func (m Model) fanCard(width int, src sourceState) string {
	lines := []string{styleAccent().Render("fans")}
	fans := src.snap.Fans
	sev := m.metricSev(hostOf(src), "fan")
	if len(fans) == 0 {
		hint := "no fan sensors"
		if n := firstNote(src.snap.Notes); n != "" {
			hint = "unavailable · " + n
		}
		lines = append(lines, styleDim().Render(hint))
		return m.card(width, sev, lines...)
	}
	inner := m.innerWidth(width)
	limit := len(fans)
	if limit > 6 {
		limit = 6
	}
	for _, f := range fans[:limit] {
		if m.cfg.Threshold.FanPctWarn > 0 && f.HasPercent && f.Percent >= m.cfg.Threshold.FanPctWarn {
			sev = worse(sev, sevAmber)
		}
		if m.cfg.Threshold.FanRPMWarn > 0 && f.HasRPM && f.RPM >= m.cfg.Threshold.FanRPMWarn {
			sev = worse(sev, sevAmber)
		}
		val := fanValue(f)
		nameW := inner - len([]rune(val)) - 1
		if nameW < 6 {
			nameW = 6
		}
		name := trunc(f.Name, nameW)
		lines = append(lines, styleText().Render(pad(name, nameW))+" "+styleDim().Render(val))
	}
	return m.card(width, sev, lines...)
}

func (m Model) tempCard(width int, src sourceState) string {
	lines := []string{styleAccent().Render("temps")}
	if hasFinite(src.hist.temp) {
		lines = append(lines, m.trend("temp", src.hist.temp, m.innerWidth(width)))
	}
	temps := src.snap.Temps
	if len(temps) == 0 {
		hint := "no temperature sensors"
		if n := firstNote(src.snap.Notes); n != "" {
			hint = "unavailable · " + n
		}
		lines = append(lines, styleDim().Render(hint))
		return m.card(width, sevOK, lines...)
	}
	sev := m.metricSev(hostOf(src), "temp")
	inner := m.innerWidth(width)
	limit := len(temps)
	if limit > 8 {
		limit = 8
	}
	for _, t := range temps[:limit] {
		sev = worse(sev, m.tempSev(t.Celsius))
		name := trunc(t.Name, max(8, inner-10))
		lines = append(lines, styleText().Render(pad(name, max(8, inner-10)))+" "+m.tempText(t.Celsius))
	}
	return m.card(width, sev, lines...)
}

func (m Model) diskCard(width int, src sourceState) string {
	lines := []string{styleAccent().Render("disks")}
	disks := src.snap.Disks
	sev := m.metricSev(hostOf(src), "disk")
	warn := m.cfg.Threshold.DiskWarn
	if warn <= 0 {
		warn = 101
	}
	if len(disks) == 0 {
		lines = append(lines, styleDim().Render("no disks"))
		return m.card(width, sev, lines...)
	}
	inner := m.innerWidth(width)
	limit := len(disks)
	if limit > 4 {
		limit = 4
	}
	for _, d := range disks[:limit] {
		if m.cfg.Threshold.DiskWarn > 0 && d.UsedPercent >= m.cfg.Threshold.DiskWarn {
			sev = worse(sev, sevAmber)
		}
		name := trunc(d.Mount, max(6, inner/3))
		line := styleText().Render(pad(name, max(6, inner/3))) + " " + m.pctColored(d.UsedPercent, warn)
		if d.ThroughputOK {
			line += styleDim().Render(fmt.Sprintf("  r %s  w %s", metrics.FormatRate(d.ReadBytesPerSec), metrics.FormatRate(d.WriteBytesPerSec)))
		}
		lines = append(lines, ansi.Truncate(line, inner+8, "…"))
	}
	return m.card(width, sev, lines...)
}

func (m Model) netCard(width int, src sourceState) string {
	lines := []string{styleAccent().Render("network")}
	nics := src.snap.Network
	if len(nics) == 0 {
		lines = append(lines, styleDim().Render("no interfaces"))
		return m.card(width, sevOK, lines...)
	}
	limit := len(nics)
	if limit > 4 {
		limit = 4
	}
	for _, n := range nics[:limit] {
		lines = append(lines, styleText().Render(n.Name)+styleDim().Render(fmt.Sprintf("  ↓ %s  ↑ %s", metrics.FormatRate(n.RxBytesPerSec), metrics.FormatRate(n.TxBytesPerSec))))
	}
	return m.card(width, sevOK, lines...)
}

func (m Model) trend(label string, vals []float64, width int) string {
	w := width - len(label) - 1
	if w < 1 {
		w = 1
	}
	spark := Sparkline(vals, w)
	ink := styleValue(colorAccent)
	if m.current().stale {
		ink = styleDim()
	}
	return styleDim().Render(label) + " " + ink.Render(spark)
}

func (m Model) card(width int, sev severity, lines ...string) string {
	border := colorBorder
	if m.current().stale {
		border = colorAmber
	} else {
		switch sev {
		case sevAmber:
			border = colorAmber
		case sevRed:
			border = colorRed
		}
	}
	st := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(border)).
		Padding(0, 1)
	frame := st.GetHorizontalFrameSize()
	inner := width - frame
	if inner < 8 {
		inner = 8
	}
	return st.Width(inner).Render(strings.Join(lines, "\n"))
}

func (m Model) innerWidth(width int) int {
	// lipgloss Width includes padding, so the text column is frame + padding
	// narrower than the outer card.
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	text := width - st.GetHorizontalFrameSize() - 2
	if text < 8 {
		return 8
	}
	return text
}

func fanValue(f metrics.Fan) string {
	var parts []string
	if f.HasRPM {
		parts = append(parts, fmt.Sprintf("%.0f rpm", f.RPM))
	}
	if f.HasPercent {
		parts = append(parts, metrics.FormatPercent(f.Percent))
	}
	if len(parts) == 0 {
		return "unavailable"
	}
	return strings.Join(parts, "  ")
}

func (m Model) coreLines(cores []float64, width int) []string {
	if len(cores) == 0 {
		return nil
	}
	shown := cores
	extra := 0
	if len(shown) > 8 {
		extra = len(shown) - 8
		shown = shown[:8]
	}
	barW := 8
	if width > 50 {
		barW = 12
	}
	var lines []string
	for i := 0; i < len(shown); i += 2 {
		left := m.coreCell(i, shown[i], barW)
		right := ""
		if i+1 < len(shown) {
			right = "  " + m.coreCell(i+1, shown[i+1], barW)
		}
		lines = append(lines, left+right)
	}
	if extra > 0 {
		lines = append(lines, styleDim().Render(fmt.Sprintf("+%d cores", extra)))
	}
	return lines
}

func (m Model) coreCell(i int, pct float64, barW int) string {
	return styleDim().Render(fmt.Sprintf("%2d ", i)) + m.bar(pct, barW, m.cfg.Threshold.CPUWarn, 98) + " " + styleText().Render(metrics.FormatPercent(pct))
}

func (m Model) bar(pct float64, width int, warn, crit float64) string {
	if width < 1 {
		width = 1
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(math.Round(pct / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	color := colorOK
	if pct >= crit {
		color = colorRed
	} else if pct >= warn {
		color = colorAmber
	}
	return styleValue(color).Render(strings.Repeat("█", filled)) + styleValue(colorTrack).Render(strings.Repeat("░", width-filled))
}

func (m Model) tempSev(c float64) severity {
	if c >= m.cfg.Threshold.TempRed {
		return sevRed
	}
	if c >= m.cfg.Threshold.TempAmber {
		return sevAmber
	}
	return sevOK
}

func (m Model) tempText(c float64) string {
	return styleValue(m.tempHex(c)).Render(metrics.FormatFixed(c, "°C"))
}

func (m Model) tempPlain(c float64) string {
	return m.tempText(c)
}

func (m Model) tempHex(c float64) string {
	switch m.tempSev(c) {
	case sevRed:
		return colorRed
	case sevAmber:
		return colorAmber
	default:
		return colorOK
	}
}

func (m Model) pctColored(v, warn float64) string {
	color := colorOK
	if v >= 98 {
		color = colorRed
	} else if v >= warn {
		color = colorAmber
	}
	return styleValue(color).Render(metrics.FormatPercent(v))
}

func labeledGauge(label string, g metrics.Gauge, format func(float64) string) string {
	if !g.Available {
		return styleDim().Render(label + " unavailable")
	}
	return styleDim().Render(label+" ") + styleText().Render(format(g.Value))
}

func fanText(g metrics.GPU) string {
	switch {
	case g.FanRPM.Available && g.FanPercent.Available:
		return styleText().Render(fmt.Sprintf("%.0f rpm", g.FanRPM.Value)+"  ") + styleDim().Render(metrics.FormatPercent(g.FanPercent.Value))
	case g.FanRPM.Available:
		return styleText().Render(fmt.Sprintf("%.0f rpm", g.FanRPM.Value))
	case g.FanPercent.Available:
		return styleText().Render(metrics.FormatPercent(g.FanPercent.Value))
	case g.FanPercent.Hint != "" || g.FanRPM.Hint != "":
		return styleDim().Render("fan unavailable")
	default:
		return styleDim().Render("fan unavailable")
	}
}

func hostOf(src sourceState) string {
	if src.snap.Host.Hostname != "" {
		return src.snap.Host.Hostname
	}
	if src.name != "" {
		return src.name
	}
	return ""
}

func (m Model) activeEvents(host string) []alert.Event {
	if !m.cfg.Alerts.Enabled || m.alerts == nil {
		return nil
	}
	var out []alert.Event
	for _, ev := range m.alerts.Active() {
		if ev.Host == host {
			out = append(out, ev)
		}
	}
	return out
}

func (m Model) alertBanner(host string) string {
	evs := m.activeEvents(host)
	if len(evs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(evs))
	red := false
	for _, ev := range evs {
		if ev.Level == "red" {
			red = true
		}
		text := ev.Summary()
		if ev.Kind == "baseline" {
			text = "base " + text
		}
		parts = append(parts, text)
	}
	label := "alert"
	if len(evs) > 1 {
		label = "alerts"
	}
	color := colorAmber
	if red {
		color = colorRed
	}
	line := label + "  " + strings.Join(parts, " · ")
	return styleValue(color).Render(ansi.Truncate(line, max(8, m.width), "…"))
}

func (m Model) alertStatus(host string) string {
	evs := m.activeEvents(host)
	n := len(evs)
	if n == 0 {
		return ""
	}
	word := "alert"
	if n != 1 {
		word = "alerts"
	}
	color := colorAmber
	for _, ev := range evs {
		if ev.Level == "red" {
			color = colorRed
			break
		}
	}
	return styleValue(color).Render(fmt.Sprintf("%d %s", n, word))
}

func (m Model) metricSev(host, metric string) severity {
	sev := sevOK
	for _, ev := range m.activeEvents(host) {
		if ev.Kind != "baseline" || ev.Metric != metric {
			continue
		}
		if ev.Level == "red" {
			return sevRed
		}
		sev = sevAmber
	}
	return sev
}

func (m Model) keySev(host, key string) severity {
	sev := sevOK
	for _, ev := range m.activeEvents(host) {
		if ev.Kind != "baseline" || ev.Key != key {
			continue
		}
		if ev.Level == "red" {
			return sevRed
		}
		sev = sevAmber
	}
	return sev
}

func worse(a, b severity) severity {
	if b > a {
		return b
	}
	return a
}

func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func pad(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

func firstNote(notes []string) string {
	notes = metrics.DedupNotes(notes)
	if len(notes) == 0 {
		return ""
	}
	return notes[0]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
