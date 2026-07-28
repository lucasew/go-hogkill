package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasew/go-hogkill/internal/kill"
	"github.com/lucasew/go-hogkill/internal/proc"
	"github.com/lucasew/go-hogkill/internal/render"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// Options configure the interactive view.
type Options struct {
	Interval      time.Duration
	Sort          proc.SortKey
	MinCPU        float64
	MinMem        uint64
	User          string
	Filter        string
	SafeOnly      bool
	DryRun        bool
	EscalateAfter time.Duration
}

type mode int

const (
	modeList mode = iota
	modeFilter
	modeConfirm
	modeHelp
)

type rowKind int

const (
	rowGroup rowKind = iota
	rowProc
)

type row struct {
	kind  rowKind
	id    string
	group proc.Group
	p     proc.Proc
}

type confirmState struct {
	targets  []kill.Target
	subject  string
	label    string
	force    bool
	risk     proc.RiskLevel
	warnings []proc.Warning
}

type sampleMsg struct {
	procs []proc.Proc
	err   error
}

type tickMsg time.Time

type killDoneMsg struct {
	summary string
}

type model struct {
	opts       Options
	sampler    *proc.Sampler
	groups     []proc.Group
	rows       []row
	order      []string
	procOrd    map[string][]int32
	cursor     int
	offset     int
	selected   map[string]struct{}
	expanded   map[string]struct{}
	sort       proc.SortKey
	pinned     bool
	paused     bool
	filter     string
	mode       mode
	confirm    *confirmState
	width      int
	height     int
	toast      string
	toastUntil time.Time
	procCount  int
	filterIn   textinput.Model
	busy       bool
	totalMem   uint64
}

// Run starts the interactive program.
func Run(opts Options) error {
	if opts.Interval <= 0 {
		opts.Interval = 1500 * time.Millisecond
	}
	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 128
	ti.Width = 40
	ti.SetValue(opts.Filter)

	var total uint64
	if vm, err := mem.VirtualMemory(); err == nil {
		total = vm.Total
	}

	m := model{
		opts:     opts,
		sampler:  &proc.Sampler{},
		sort:     opts.Sort,
		filter:   opts.Filter,
		selected: map[string]struct{}{},
		expanded: map[string]struct{}{},
		procOrd:  map[string][]int32{},
		filterIn: ti,
		width:    80,
		height:   24,
		totalMem: total,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.sampleCmd(), m.tickCmd())
}

func (m model) tickCmd() tea.Cmd {
	return tea.Tick(m.opts.Interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) sampleCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		procs, err := m.sampler.Sample(ctx)
		return sampleMsg{procs: procs, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		if m.paused || m.busy {
			return m, m.tickCmd()
		}
		m.busy = true
		return m, tea.Batch(m.sampleCmd(), m.tickCmd())

	case sampleMsg:
		m.busy = false
		if msg.err != nil {
			m.flash("sample failed: " + msg.err.Error())
			return m, nil
		}
		m.applySample(msg.procs)
		return m, nil

	case killDoneMsg:
		m.selected = map[string]struct{}{}
		m.flash(msg.summary)
		m.busy = true
		return m, m.sampleCmd()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeList {
		// click dismisses help; ignore elsewhere
		if m.mode == modeHelp && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.mode = modeList
		}
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if msg.Action == tea.MouseActionPress {
			m.move(-3)
		}
		return m, nil
	case tea.MouseButtonWheelDown:
		if msg.Action == tea.MouseActionPress {
			m.move(3)
		}
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// Layout: 0 header, 1 rule, 2 columns, 3.. body
	const colRow = 2
	const bodyStart = 3
	if msg.Y == 0 && m.canTogglePause() {
		if hl := m.headerLayout(); msg.X >= hl.stateStart && msg.X < hl.stateEnd {
			return m.togglePause()
		}
		return m, nil
	}
	if msg.Y == colRow {
		if key, ok := m.colLayout().hit(msg.X); ok {
			m.reorder(key)
			m.flash("sort " + string(key))
		}
		return m, nil
	}
	if msg.Y >= bodyStart {
		body := m.viewportHeight()
		offset := m.scrollOffset(body)
		row := offset + (msg.Y - bodyStart)
		if row >= 0 && row < len(m.rows) {
			m.cursor = row
			m.offset = offset
			// keep cursor visible after click
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
			if m.cursor >= m.offset+body {
				m.offset = m.cursor - body + 1
			}
		}
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeFilter {
		switch msg.String() {
		case "enter":
			m.mode = modeList
			m.filter = m.filterIn.Value()
			m.cursor = 0
			m.order = nil
			m.busy = true
			return m, m.sampleCmd()
		case "esc":
			m.mode = modeList
			m.filterIn.SetValue(m.filter)
			return m, nil
		}
		var cmd tea.Cmd
		m.filterIn, cmd = m.filterIn.Update(msg)
		// live filter
		m.filter = m.filterIn.Value()
		m.cursor = 0
		m.order = nil
		m.busy = true
		return m, tea.Batch(cmd, m.sampleCmd())
	}

	if m.mode == modeConfirm {
		switch msg.String() {
		case "y", "Y", "enter":
			return m, m.executeKill(false)
		case "k", "K":
			return m, m.executeKill(true)
		default:
			m.confirm = nil
			m.mode = modeList
		}
		return m, nil
	}

	if m.mode == modeHelp {
		m.mode = modeList
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "pgdown":
		m.move(m.viewportHeight())
	case "pgup":
		m.move(-m.viewportHeight())
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		if len(m.rows) > 0 {
			m.cursor = len(m.rows) - 1
		}
	case "l", "right", "enter":
		m.toggleExpand(true)
	case "h", "left":
		m.toggleExpand(false)
	case " ":
		m.toggleSelect()
	case "a":
		for _, r := range m.rows {
			if r.kind == rowGroup {
				m.selected[r.id] = struct{}{}
			}
		}
	case "x":
		m.selected = map[string]struct{}{}
	case "d":
		m.requestKill(false)
		return m, nil
	case "D":
		m.requestKill(true)
		return m, nil
	case "p":
		m.pinned = !m.pinned
		if m.pinned {
			m.flash("order pinned")
		} else {
			m.flash("order live again")
		}
	case "s":
		m.cycleSort()
		m.flash("sort " + string(m.sort))
	case "c":
		m.reorder(proc.SortCPU)
		m.flash("sort cpu")
	case "m":
		m.reorder(proc.SortMem)
		m.flash("sort mem")
	case "/":
		m.mode = modeFilter
		m.filterIn.SetValue(m.filter)
		m.filterIn.Focus()
		return m, textinput.Blink
	case "r":
		m.reorder(m.sort)
		m.busy = true
		return m, m.sampleCmd()
	case "?":
		m.mode = modeHelp
	}
	return m, nil
}

func (m *model) held() bool {
	return m.pinned || m.cursor > 0 || len(m.selected) > 0 || m.mode != modeList
}

func (m model) stateText() string {
	if m.paused {
		return "⏸  paused"
	}
	if m.held() {
		if m.pinned {
			return "pinned"
		}
		return "held"
	}
	return "⬤ live"
}

func (m model) headerRightPlain() string {
	right := fmt.Sprintf("  %s · sort %s", m.stateText(), m.sort)
	if m.filter != "" {
		right += " · /" + m.filter
	}
	return right
}

func (m model) renderState(stateText string, color lipgloss.Color) string {
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(stateText)
}

func (m model) canTogglePause() bool {
	return m.mode == modeList && (m.paused || !m.held())
}

type headerLayout struct {
	stateStart int
	stateEnd   int
}

func (m model) headerLayout() headerLayout {
	right := m.headerRightPlain()
	state := m.stateText()
	rightW := lipgloss.Width(right)
	stateW := lipgloss.Width(state)
	// state sits after two leading spaces on the right segment
	start := m.width - rightW + 2
	return headerLayout{stateStart: start, stateEnd: start + stateW}
}

func (m model) togglePause() (tea.Model, tea.Cmd) {
	m.paused = !m.paused
	if !m.paused {
		m.busy = true
		return m, tea.Batch(m.sampleCmd(), m.tickCmd())
	}
	return m, nil
}

func (m *model) applySample(procs []proc.Proc) {
	m.procCount = len(procs)
	grouped := proc.GroupProcesses(procs, proc.GroupOptions{
		MinCPU:   m.opts.MinCPU,
		MinMem:   m.opts.MinMem,
		User:     m.opts.User,
		Filter:   m.filter,
		SafeOnly: m.opts.SafeOnly,
	})
	var ordered []proc.Group
	if m.held() {
		ordered = m.applyOrder(grouped)
		for i := range ordered {
			m.applyProcOrder(&ordered[i])
		}
	} else {
		ordered = proc.SortGroups(grouped, m.sort)
	}
	m.rememberOrder(ordered)
	m.groups = ordered
	m.rebuildRows()
}

func (m *model) applyOrder(groups []proc.Group) []proc.Group {
	byKey := make(map[string]proc.Group, len(groups))
	for _, g := range groups {
		byKey[g.Key] = g
	}
	var kept []proc.Group
	for _, key := range m.order {
		if g, ok := byKey[key]; ok {
			kept = append(kept, g)
			delete(byKey, key)
		}
	}
	rest := make([]proc.Group, 0, len(byKey))
	for _, g := range byKey {
		rest = append(rest, g)
	}
	rest = proc.SortGroups(rest, m.sort)
	return append(kept, rest...)
}

func (m *model) applyProcOrder(g *proc.Group) {
	remembered := m.procOrd[g.Key]
	if len(remembered) == 0 {
		return
	}
	byPID := make(map[int32]proc.Proc, len(g.Procs))
	for _, p := range g.Procs {
		byPID[p.PID] = p
	}
	var kept []proc.Proc
	for _, pid := range remembered {
		if p, ok := byPID[pid]; ok {
			kept = append(kept, p)
			delete(byPID, pid)
		}
	}
	for _, p := range byPID {
		kept = append(kept, p)
	}
	g.Procs = kept
}

func (m *model) rememberOrder(groups []proc.Group) {
	m.order = make([]string, len(groups))
	m.procOrd = make(map[string][]int32, len(groups))
	for i, g := range groups {
		m.order[i] = g.Key
		pids := make([]int32, len(g.Procs))
		for j, p := range g.Procs {
			pids[j] = p.PID
		}
		m.procOrd[g.Key] = pids
	}
}

func (m *model) rebuildRows() {
	var anchor string
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		anchor = m.rows[m.cursor].id
	}
	var rows []row
	for _, g := range m.groups {
		rows = append(rows, row{kind: rowGroup, id: "g:" + g.Key, group: g})
		if _, ok := m.expanded[g.Key]; ok {
			for _, p := range g.Procs {
				rows = append(rows, row{kind: rowProc, id: fmt.Sprintf("p:%d", p.PID), group: g, p: p})
			}
		}
	}
	m.rows = rows
	if anchor != "" {
		for i, r := range rows {
			if r.id == anchor {
				m.cursor = i
				break
			}
		}
	}
	if m.cursor >= len(rows) {
		m.cursor = max(0, len(rows)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) move(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = clamp(m.cursor+delta, 0, len(m.rows)-1)
	body := m.viewportHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+body {
		m.offset = m.cursor - body + 1
	}
}

func (m *model) toggleExpand(open bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	key := r.group.Key
	if open {
		if r.kind == rowProc {
			return
		}
		m.expanded[key] = struct{}{}
	} else if r.kind == rowProc {
		delete(m.expanded, key)
		for i, row := range m.rows {
			if row.id == "g:"+key {
				m.cursor = i
				break
			}
		}
	} else {
		delete(m.expanded, key)
	}
	m.rebuildRows()
}

func (m *model) toggleSelect() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	id := m.rows[m.cursor].id
	if _, ok := m.selected[id]; ok {
		delete(m.selected, id)
	} else {
		m.selected[id] = struct{}{}
	}
	m.move(1)
}

func (m *model) cycleSort() {
	cycle := []proc.SortKey{proc.SortCPU, proc.SortMem, proc.SortCount, proc.SortName}
	idx := 0
	for i, s := range cycle {
		if s == m.sort {
			idx = (i + 1) % len(cycle)
			break
		}
	}
	m.reorder(cycle[idx])
}

func (m *model) reorder(sort proc.SortKey) {
	m.sort = sort
	m.groups = proc.SortGroups(m.groups, sort)
	for i := range m.groups {
		// procs stay cpu-sorted
		ps := m.groups[i].Procs
		for a := 0; a < len(ps); a++ {
			for b := a + 1; b < len(ps); b++ {
				if ps[b].CPU > ps[a].CPU || (ps[b].CPU == ps[a].CPU && ps[b].RSS > ps[a].RSS) {
					ps[a], ps[b] = ps[b], ps[a]
				}
			}
		}
		m.groups[i].Procs = ps
	}
	m.rememberOrder(m.groups)
	m.rebuildRows()
}

func (m *model) flash(msg string) {
	m.toast = msg
	m.toastUntil = time.Now().Add(5 * time.Second)
}

func (m *model) requestKill(force bool) {
	var chosen []row
	for _, r := range m.rows {
		if _, ok := m.selected[r.id]; ok {
			chosen = append(chosen, r)
		}
	}
	if len(chosen) == 0 && m.cursor >= 0 && m.cursor < len(m.rows) {
		chosen = []row{m.rows[m.cursor]}
	}
	if len(chosen) == 0 {
		return
	}
	procs := map[int32]proc.Proc{}
	apps := map[string]struct{}{}
	for _, r := range chosen {
		apps[r.group.Name] = struct{}{}
		if r.kind == rowGroup {
			for _, p := range r.group.Procs {
				procs[p.PID] = p
			}
		} else {
			procs[r.p.PID] = r.p
		}
	}
	list := make([]proc.Proc, 0, len(procs))
	var reclaimed uint64
	targets := make([]kill.Target, 0, len(procs))
	for _, p := range procs {
		list = append(list, p)
		reclaimed += p.RSS
		targets = append(targets, kill.Target{
			PID:  p.PID,
			Name: fmt.Sprintf("%s (%d)", p.Name, p.PID),
			Own:  p.Risk == proc.RiskOwn,
		})
	}
	subject := fmt.Sprintf("%d apps", len(apps))
	if len(apps) == 1 {
		for n := range apps {
			subject = n
		}
	}
	label := fmt.Sprintf("%s · %d process%s · %s", subject, len(list), plural(len(list)), render.Bytes(reclaimed))
	m.confirm = &confirmState{
		targets:  targets,
		subject:  subject,
		label:    label,
		force:    force,
		risk:     proc.HighestRisk(list),
		warnings: proc.CollectWarnings(list),
	}
	m.mode = modeConfirm
}

func (m *model) executeKill(forceOverride bool) tea.Cmd {
	if m.confirm == nil {
		return nil
	}
	pending := *m.confirm
	if forceOverride {
		pending.force = true
	}
	m.confirm = nil
	m.mode = modeList
	m.toast = fmt.Sprintf("killing %s (%d)…", pending.subject, len(pending.targets))
	m.toastUntil = time.Now().Add(10 * time.Second)

	esc := m.opts.EscalateAfter
	if pending.force {
		esc = 0
	}
	dry := m.opts.DryRun
	subject := pending.subject
	targets := pending.targets
	force := pending.force

	return func() tea.Msg {
		outcomes := kill.Targets(targets, kill.Options{
			Force:         force,
			EscalateAfter: esc,
			DryRun:        dry,
		})
		return killDoneMsg{summary: kill.Summarize(outcomes, subject)}
	}
}

func (m model) viewportHeight() int {
	return max(1, m.height-6)
}

func (m model) scrollOffset(body int) int {
	offset := m.offset
	if m.cursor < offset {
		offset = m.cursor
	}
	if m.cursor >= offset+body {
		offset = m.cursor - body + 1
	}
	return clamp(offset, 0, max(0, len(m.rows)-body))
}

// colSpan is one clickable header segment.
type colSpan struct {
	key   proc.SortKey // empty = not sortable
	label string
	width int
	start int
}

type colLayout struct {
	prefix int
	spans  []colSpan
}

func (m model) colLayout() colLayout {
	withUser := m.width >= 100 && proc.SupportsUsers
	nameW := max(14, m.width-60)
	if !withUser {
		nameW = max(14, m.width-50)
	}
	// "❯[ ] " = 5 cells of chrome before NAME
	const prefix = 5
	x := prefix
	spans := []colSpan{
		{key: proc.SortName, label: "NAME", width: nameW, start: x},
	}
	x += nameW
	spans = append(spans, colSpan{key: proc.SortCPU, label: "CPU", width: 7, start: x})
	x += 7
	spans = append(spans, colSpan{key: proc.SortMem, label: "MEMORY", width: 11, start: x})
	x += 11
	spans = append(spans, colSpan{key: proc.SortCount, label: "PROCS·AGE", width: 9, start: x})
	x += 9 + 2 // gap before RISK
	spans = append(spans, colSpan{key: "", label: "RISK", width: 8, start: x})
	x += 8
	if withUser {
		x++ // space
		spans = append(spans, colSpan{key: "", label: "USER", width: 10, start: x})
	}
	return colLayout{prefix: prefix, spans: spans}
}

func (c colLayout) hit(x int) (proc.SortKey, bool) {
	for _, s := range c.spans {
		if s.key == "" {
			continue
		}
		if x >= s.start && x < s.start+s.width {
			return s.key, true
		}
	}
	return "", false
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")
	b.WriteString(m.columns())
	b.WriteString("\n")

	body := m.viewportHeight()
	if m.mode == modeHelp {
		b.WriteString(m.helpBody(body))
	} else {
		scaleCPU, scaleRSS := 1.0, uint64(1)
		for _, g := range m.groups {
			if g.CPU > scaleCPU {
				scaleCPU = g.CPU
			}
			if g.RSS > scaleRSS {
				scaleRSS = g.RSS
			}
		}
		offset := m.scrollOffset(body)

		slice := m.rows[offset:min(offset+body, len(m.rows))]
		for i, r := range slice {
			active := offset+i == m.cursor
			b.WriteString(m.renderRow(r, active, scaleCPU, scaleRSS))
			b.WriteString("\n")
		}
		for i := len(slice); i < body; i++ {
			if len(m.rows) == 0 && i == 0 {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render("  nothing matches"))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString(lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")
	b.WriteString(m.status())
	return b.String()
}

func (m model) header() string {
	stateText := m.stateText()
	rightPlain := m.headerRightPlain()
	rightW := lipgloss.Width(rightPlain)

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Render("hk")
	var state string
	switch {
	case m.paused:
		state = m.renderState(stateText, lipgloss.Color("3"))
	case m.held():
		state = m.renderState(stateText, lipgloss.Color("3"))
	default:
		state = m.renderState(stateText, lipgloss.Color("2"))
	}
	right := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("  %s · sort %s", state, m.sort))
	if m.filter != "" {
		right += lipgloss.NewStyle().Faint(true).Render(" · /" + m.filter)
	}

	var used uint64
	if vm, err := mem.VirtualMemory(); err == nil {
		used = vm.Used
		m.totalMem = vm.Total
	}
	cores := float64(proc.Cores())
	var busy float64
	for _, g := range m.groups {
		busy += g.CPU
	}
	cpuRatio := busy / (cores * 100)
	if cpuRatio > 1 {
		cpuRatio = 1
	}
	memRatio := 0.0
	if m.totalMem > 0 {
		memRatio = float64(used) / float64(m.totalMem)
	}
	loadStr := ""
	if avg, err := load.Avg(); err == nil {
		loadStr = fmt.Sprintf(" · load %.2f", avg.Load1)
	}

	stats := fmt.Sprintf("%d procs · cpu %s %s · ram %s/%s %s%s",
		m.procCount,
		render.Percent(cpuRatio*100),
		render.Bar(cpuRatio, 10),
		render.Bytes(used),
		render.Bytes(m.totalMem),
		render.Bar(memRatio, 10),
		loadStr,
	)
	const headerGap = 4
	titlePrefix := "hk  "
	maxStatsW := m.width - lipgloss.Width(titlePrefix) - rightW - headerGap
	if maxStatsW > 0 && lipgloss.Width(stats) > maxStatsW {
		stats = strings.TrimSpace(render.Fit(stats, maxStatsW))
	}
	left := title + "  " + stats
	gap := m.width - lipgloss.Width(left) - rightW
	if gap < headerGap {
		gap = headerGap
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m model) columns() string {
	layout := m.colLayout()
	active := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Underline(true)
	idle := lipgloss.NewStyle().Faint(true)
	var b strings.Builder
	b.WriteString(idle.Render(strings.Repeat(" ", layout.prefix)))
	prevEnd := layout.prefix
	for i, s := range layout.spans {
		if s.start > prevEnd {
			b.WriteString(idle.Render(strings.Repeat(" ", s.start-prevEnd)))
		}
		label := s.label
		// right-align numeric columns the way data cells do
		var cell string
		switch s.key {
		case proc.SortCPU, proc.SortMem, proc.SortCount:
			cell = render.PadStart(label, s.width)
		default:
			cell = render.Fit(label, s.width)
		}
		if s.key != "" && s.key == m.sort {
			// mark active sort with a caret on the label
			mark := "▾" + label
			if s.key == proc.SortCPU || s.key == proc.SortMem || s.key == proc.SortCount {
				cell = render.PadStart(mark, s.width)
			} else {
				cell = render.Fit(mark, s.width)
			}
			b.WriteString(active.Render(cell))
		} else if s.key != "" {
			b.WriteString(idle.Render(cell))
		} else {
			b.WriteString(idle.Render(cell))
		}
		prevEnd = s.start + s.width
		_ = i
	}
	return b.String()
}

func (m model) renderRow(r row, active bool, scaleCPU float64, scaleRSS uint64) string {
	withUser := m.width >= 100 && proc.SupportsUsers
	nameW := max(14, m.width-60)
	if !withUser {
		nameW = max(14, m.width-50)
	}

	var cpu float64
	var rss uint64
	var risk proc.RiskLevel
	var label string
	if r.kind == rowGroup {
		cpu, rss, risk = r.group.CPU, r.group.RSS, r.group.Risk
		caret := "  "
		if len(r.group.Procs) > 1 {
			if _, ok := m.expanded[r.group.Key]; ok {
				caret = "▾ "
			} else {
				caret = "▸ "
			}
		}
		label = caret + r.group.Name
	} else {
		cpu, rss, risk = r.p.CPU, r.p.RSS, r.p.Risk
		label = fmt.Sprintf("  └ %6d %s", r.p.PID, r.p.Name)
	}

	cursor := " "
	if active {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("❯")
	}
	box := lipgloss.NewStyle().Faint(true).Render("[ ]")
	if _, ok := m.selected[r.id]; ok {
		box = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("[x]")
	}

	ageOrCount := ""
	if r.kind == rowGroup {
		ageOrCount = fmt.Sprintf("%d", len(r.group.Procs))
	} else {
		ageOrCount = render.Duration(r.p.Elapsed)
	}

	riskStyle := lipgloss.NewStyle()
	switch risk {
	case proc.RiskCritical:
		riskStyle = riskStyle.Foreground(lipgloss.Color("1"))
	case proc.RiskSystem:
		riskStyle = riskStyle.Foreground(lipgloss.Color("3"))
	case proc.RiskOwn:
		riskStyle = riskStyle.Foreground(lipgloss.Color("6"))
	}

	line := cursor + box + " " +
		render.Fit(label, nameW) +
		render.PadStart(render.Percent(cpu), 7) +
		render.PadStart(render.Bytes(rss), 11) +
		render.PadStart(ageOrCount, 9) + "  " +
		riskStyle.Render(render.Fit(proc.RiskTag[risk], 8))
	if withUser {
		u := r.group.User
		if r.kind == rowProc {
			u = r.p.User
		}
		line += " " + lipgloss.NewStyle().Faint(true).Render(render.Fit(u, 10))
	}
	_ = scaleCPU
	_ = scaleRSS
	if active {
		return lipgloss.NewStyle().Background(lipgloss.Color("236")).Render(line)
	}
	return line
}

// bkey bolds the shortcut letter and leaves the rest plain, e.g. bkey("s","ort") → **s**ort
func bkey(key, rest string) string {
	return lipgloss.NewStyle().Bold(true).Render(key) + rest
}

func (m model) helpBody(height int) string {
	dim := lipgloss.NewStyle().Faint(true)
	lines := []string{
		"",
		"  navigate  ↑↓ / " + bkey("k", "") + bkey("j", "") + " move · →← / " + bkey("l", "") + bkey("h", "") + " expand · " + bkey("g", "") + " / " + bkey("G", "") + " top/bottom",
		"            click a row to focus · wheel to scroll",
		"",
		"  act       " + bkey("space", "") + " select · " + bkey("a", "") + " select all · " + bkey("x", "") + " clear",
		"            " + bkey("d", "") + " kill (SIGTERM then SIGKILL) · " + bkey("D", "") + " force kill",
		"",
		"  view      " + bkey("/", "") + " filter · " + bkey("s", "ort") + " cycle · " + bkey("c", "pu") + " · " + bkey("m", "em") + " · " + bkey("p", "in") + " · " + bkey("q", "uit"),
		"            click column headers (NAME / CPU / MEMORY / PROCS) to sort",
		"",
		"  order     live only at top with nothing selected",
		"            click live / paused in the header to pause or resume",
		"            held when you move — numbers update, rows stay",
		"",
		"  risk      critical / system / you — never blocks a kill",
		"",
		dim.Render("  press any key to go back"),
	}
	if height < len(lines) {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m model) status() string {
	if m.mode == modeConfirm && m.confirm != nil {
		var lines []string
		for i, w := range m.confirm.warnings {
			if i >= 3 {
				lines = append(lines, fmt.Sprintf("  …and %d more risky processes in this batch", len(m.confirm.warnings)-3))
				break
			}
			lines = append(lines, fmt.Sprintf("  %s %s — %s", proc.RiskTag[w.Level], w.Name, w.Reason))
		}
		verb := kill.KillVerb(m.confirm.force)
		prefix := ""
		if m.opts.DryRun {
			prefix = "[dry run] "
		}
		headline := "kill"
		if m.confirm.risk != proc.RiskNone {
			headline = "kill " + proc.RiskWord[m.confirm.risk]
		}
		keys := bkey("y", "") + " yes · " + bkey("K", "") + " force · " + bkey("n", "") + " cancel"
		lines = append(lines, fmt.Sprintf("%s%s %s · %s?  %s", prefix, headline, m.confirm.label, verb, keys))
		return strings.Join(lines, "\n")
	}
	if m.mode == modeFilter {
		return m.filterIn.View() + "  enter to keep · esc cancel"
	}
	if m.toast != "" && time.Now().Before(m.toastUntil) {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(m.toast)
	}
	sel := ""
	if len(m.selected) > 0 {
		sel = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("%d selected · ", len(m.selected)))
	}
	hints := lipgloss.NewStyle().Faint(true)
	if m.held() {
		return sel + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("rows held still") +
			hints.Render(" · ") + bkey("g", "") + hints.Render(" top to re-rank · space select · ") +
			bkey("d", "") + hints.Render(" kill · ") + bkey("/", "") + hints.Render(" filter · ") +
			bkey("?", "") + hints.Render(" help · ") + bkey("q", "") + hints.Render(" quit")
	}
	return sel +
		hints.Render("↑↓ move · → expand · space select · ") +
		bkey("d", "") + hints.Render(" kill · ") + bkey("/", "") + hints.Render(" filter · ") +
		bkey("s", "ort") + hints.Render(" · click headers · ") + bkey("?", "") + hints.Render(" help · ") + bkey("q", "") + hints.Render(" quit")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
