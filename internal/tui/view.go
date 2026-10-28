package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	if src.err != "" {
		extra = "\n" + styleValue(colorRed).Render("down") + styleDim().Render("  "+src.err)
	} else if notes := metrics.DedupNotes(snap.Notes); len(notes) > 0 {
		extra = "\n" + styleDim().Render(ansi.Truncate(strings.Join(notes, " · "), m.width, "…"))
	}
	rule := styleFaint().Render(strings.Repeat("─", max(1, m.width)))
	return line + extra + "\n" + rule
}

func (m Model) renderFooter() string {
	return m.help.View(m.keys)
}

func (m Model) renderHelp() string {
	th := m.cfg.Threshold
	lines := []string{
		styleAccent().Render("keys"),
		styleDim().Render("q quit    esc back    tab host    p processes"),
		styleDim().Render("c sort by cpu    m sort by memory    ? close help"),
		"",
		styleAccent().Render("thresholds"),
		styleDim().Render(fmt.Sprintf("temp amber %.0f°C    red %.0f°C", th.TempAmber, th.TempRed)),
		styleDim().Render(fmt.Sprintf("cpu %.0f%%    memory %.0f%%    gpu %.0f%%", th.CPUWarn, th.MemWarn, th.GPUUtilWarn)),
		"",
		styleDim().Render("A missing sensor stays on screen with a hint. It does not stop the view."),
	}
	if m.cfg.Alerts.Bell || m.cfg.Alerts.Command != "" {
		lines = append(lines, styleDim().Render("alerts fire once when a threshold is crossed"))
	}
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
	title := styleAccent().Render("processes")
	if m.sortMem {
		title += styleDim().Render("  by memory")
	} else {
		title += styleDim().Render("  by cpu")
	}
	return title + "\n" + m.table.View()
}

func (m Model) renderDash() string {
	src := m.current()
	if !src.seen && src.err == "" {
		return styleDim().Render("sampling…")
	}
	if !src.seen {
		return styleDim().Render("no sample yet")
	}
	if m.width >= 100 {
		gap := 1
		col := (m.width - gap) / 2
		left := lipgloss.JoinVertical(lipgloss.Left,
			m.cpuCard(col, src),
			m.memCard(col, src),
			m.diskCard(col, src),
			m.netCard(col, src),
		)
		right := lipgloss.JoinVertical(lipgloss.Left,
			m.gpuCard(col, src),
			m.fanCard(col, src),
			m.tempCard(col, src),
		)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}
	w := m.width
	return lipgloss.JoinVertical(lipgloss.Left,
		m.cpuCard(w, src),
		m.memCard(w, src),
		m.gpuCard(w, src),
		m.fanCard(w, src),
		m.tempCard(w, src),
		m.diskCard(w, src),
		m.netCard(w, src),
	)
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
	lines = append(lines, styleValue(colorAccent).Render(Sparkline(src.hist.cpu, inner)))
	lines = append(lines, m.coreLines(snap.CPU.PerCore, inner)...)
	return m.card(width, sev, lines...)
}

func (m Model) memCard(width int, src sourceState) string {
	mem := src.snap.Memory
	sev := sevOK
	if mem.UsedPercent >= m.cfg.Threshold.MemWarn {
		sev = sevAmber
	}
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
	lines = append(lines, styleValue(colorAccent).Render(Sparkline(src.hist.mem, inner)))
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
		if i == 0 {
			lines = append(lines, styleValue(colorAccent).Render(Sparkline(src.hist.gpu, inner)))
		}
		if g.Util.Available && g.Util.Value >= m.cfg.Threshold.GPUUtilWarn {
			sev = worse(sev, sevAmber)
		}
	}
	if len(gpus) > limit {
		lines = append(lines, styleDim().Render(fmt.Sprintf("+%d more", len(gpus)-limit)))
	}
	return m.card(width, sev, lines...)
}

func (m Model) fanCard(width int, src sourceState) string {
	lines := []string{styleAccent().Render("fans")}
	fans := src.snap.Fans
	if len(fans) == 0 {
		hint := "no fan sensors"
		if n := firstNote(src.snap.Notes); n != "" {
			hint = "unavailable · " + n
		}
		lines = append(lines, styleDim().Render(hint))
		return m.card(width, sevOK, lines...)
	}
	inner := m.innerWidth(width)
	limit := len(fans)
	if limit > 6 {
		limit = 6
	}
	for _, f := range fans[:limit] {
		val := fanValue(f)
		nameW := inner - len([]rune(val)) - 1
		if nameW < 6 {
			nameW = 6
		}
		name := trunc(f.Name, nameW)
		lines = append(lines, styleText().Render(pad(name, nameW))+" "+styleDim().Render(val))
	}
	return m.card(width, sevOK, lines...)
}

func (m Model) tempCard(width int, src sourceState) string {
	lines := []string{styleAccent().Render("temps")}
	temps := src.snap.Temps
	if len(temps) == 0 {
		hint := "no temperature sensors"
		if n := firstNote(src.snap.Notes); n != "" {
			hint = "unavailable · " + n
		}
		lines = append(lines, styleDim().Render(hint))
		return m.card(width, sevOK, lines...)
	}
	sev := sevOK
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
	if len(disks) == 0 {
		lines = append(lines, styleDim().Render("no disks"))
		return m.card(width, sevOK, lines...)
	}
	inner := m.innerWidth(width)
	limit := len(disks)
	if limit > 4 {
		limit = 4
	}
	for _, d := range disks[:limit] {
		name := trunc(d.Mount, max(6, inner/3))
		line := styleText().Render(pad(name, max(6, inner/3))) + " " + m.pctColored(d.UsedPercent, 90)
		if d.ThroughputOK {
			line += styleDim().Render(fmt.Sprintf("  r %s  w %s", metrics.FormatRate(d.ReadBytesPerSec), metrics.FormatRate(d.WriteBytesPerSec)))
		}
		lines = append(lines, ansi.Truncate(line, inner+8, "…"))
	}
	return m.card(width, sevOK, lines...)
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

func (m Model) card(width int, sev severity, lines ...string) string {
	border := colorBorder
	switch sev {
	case sevAmber:
		border = colorAmber
	case sevRed:
		border = colorRed
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
