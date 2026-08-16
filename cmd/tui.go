package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// ---------- tabs ----------

const (
	tabBlocks = iota
	tabGoals
	tabSleep
	tabMetrics
	tabCount
)

var tabNames = [tabCount]string{"Blocks", "Goals", "Sleep", "Metrics"}

// ---------- styles ----------

var (
	activeTabStyle   = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(lipgloss.Color("212")).Underline(true)
	inactiveTabStyle = lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("245"))
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	bannerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).
				Border(lipgloss.RoundedBorder()).Padding(0, 1).MarginTop(1).MarginBottom(1)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

// ---------- data rows ----------

type tuiBlock struct {
	id, blockNum int
	date, day    string
	project      string
	outcome      string
	focus        *int
	closed       bool
}

type tuiGoal struct {
	id, num int
	day     string
	goal    string
	done    bool
}

type tuiSleep struct {
	date          string
	hours         float64
	quality, feel int
}

type tuiMetric struct {
	project  string
	count    int
	avgFocus float64
}

type tuiProject struct {
	id   int
	name string
}

// ---------- close-block form ----------

var closeFieldLabels = []string{"Done", "Not done", "Next step", "Files/links", "Focus (1-5)", "Tweak"}

type closeFormState struct {
	blockIdx int // index into m.blocks
	fields   [6]string
	field    int
	errMsg   string
}

// ---------- start-block form ----------

var startFieldLabels = []string{"Outcome", "Context reload", "First action"}

type startFormState struct {
	projectID   int
	projectName string
	fields      [3]string
	field       int
	errMsg      string
}

// ---------- model ----------

type tuiModel struct {
	tab int

	blocks   []tuiBlock
	blockCur int

	goals   []tuiGoal
	goalCur int

	sleep []tuiSleep

	metrics []tuiMetric

	projects   []tuiProject
	projectCur int

	mode      string // "browse" | "close" | "start_project" | "start_fields"
	form      closeFormState
	startForm startFormState
	status    string
	err       error

	width, height int
}

func newTUIModel() (tuiModel, error) {
	m := tuiModel{mode: "browse"}
	if err := m.reload(); err != nil {
		return tuiModel{}, err
	}
	return m, nil
}

func (m *tuiModel) reload() error {
	weekStart := mondayOf(time.Now()).Format("2006-01-02")

	blocks, err := loadTUIBlocks(weekStart)
	if err != nil {
		return err
	}
	m.blocks = blocks
	if m.blockCur >= len(m.blocks) {
		m.blockCur = len(m.blocks) - 1
	}
	if m.blockCur < 0 {
		m.blockCur = 0
	}

	goals, err := loadTUIGoals(weekStart)
	if err != nil {
		return err
	}
	m.goals = goals
	if m.goalCur >= len(m.goals) {
		m.goalCur = len(m.goals) - 1
	}
	if m.goalCur < 0 {
		m.goalCur = 0
	}

	sleep, err := loadTUISleep(weekStart)
	if err != nil {
		return err
	}
	m.sleep = sleep

	metrics, err := loadTUIMetrics(weekStart)
	if err != nil {
		return err
	}
	m.metrics = metrics

	projects, err := loadTUIProjects()
	if err != nil {
		return err
	}
	m.projects = projects
	if m.projectCur >= len(m.projects) {
		m.projectCur = len(m.projects) - 1
	}
	if m.projectCur < 0 {
		m.projectCur = 0
	}

	return nil
}

func loadTUIBlocks(weekStart string) ([]tuiBlock, error) {
	rows, err := conn.Query(`
		SELECT b.id, b.date, b.block_num, b.day, p.name, b.outcome, b.focus_quality, b.closed_at
		FROM blocks b LEFT JOIN projects p ON p.id = b.project_id
		WHERE b.date >= ?
		ORDER BY b.date DESC, b.block_num DESC`, weekStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tuiBlock
	for rows.Next() {
		var b tuiBlock
		var project *string
		var focus *int
		var closedAt *string
		if err := rows.Scan(&b.id, &b.date, &b.blockNum, &b.day, &project, &b.outcome, &focus, &closedAt); err != nil {
			return nil, err
		}
		if project != nil {
			b.project = *project
		} else {
			b.project = "-"
		}
		b.focus = focus
		b.closed = closedAt != nil
		out = append(out, b)
	}
	return out, rows.Err()
}

func loadTUIGoals(weekStart string) ([]tuiGoal, error) {
	rows, err := conn.Query(
		`SELECT id, day, goal, done FROM weekly_goals WHERE week_start = ? ORDER BY id`,
		weekStart,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tuiGoal
	n := 0
	for rows.Next() {
		n++
		var g tuiGoal
		if err := rows.Scan(&g.id, &g.day, &g.goal, &g.done); err != nil {
			return nil, err
		}
		g.num = n
		out = append(out, g)
	}
	return out, rows.Err()
}

func loadTUISleep(weekStart string) ([]tuiSleep, error) {
	rows, err := conn.Query(
		`SELECT date, sleep_hours, sleep_quality, feel FROM daily_checkin WHERE date >= ? ORDER BY date`,
		weekStart,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tuiSleep
	for rows.Next() {
		var s tuiSleep
		if err := rows.Scan(&s.date, &s.hours, &s.quality, &s.feel); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func loadTUIMetrics(weekStart string) ([]tuiMetric, error) {
	rows, err := conn.Query(`
		SELECT p.name, COUNT(*), AVG(b.focus_quality)
		FROM blocks b JOIN projects p ON p.id = b.project_id
		WHERE b.date >= ?
		GROUP BY p.name ORDER BY COUNT(*) DESC`, weekStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tuiMetric
	for rows.Next() {
		var mt tuiMetric
		var avg *float64
		if err := rows.Scan(&mt.project, &mt.count, &avg); err != nil {
			return nil, err
		}
		if avg != nil {
			mt.avgFocus = *avg
		}
		out = append(out, mt)
	}
	return out, rows.Err()
}

func loadTUIProjects() ([]tuiProject, error) {
	rows, err := conn.Query(`SELECT id, name FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tuiProject
	for rows.Next() {
		var p tuiProject
		if err := rows.Scan(&p.id, &p.name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------- tea.Model ----------

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		m.status = ""
		switch m.mode {
		case "close":
			return m.updateCloseForm(msg)
		case "start_project":
			return m.updateStartProject(msg)
		case "start_fields":
			return m.updateStartFields(msg)
		}
		return m.updateBrowse(msg)
	}
	return m, nil
}

func (m tuiModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab":
		m.tab = (m.tab + 1) % tabCount
		return m, nil
	case "shift+tab":
		m.tab = (m.tab - 1 + tabCount) % tabCount
		return m, nil
	case "1", "2", "3", "4":
		n, _ := strconv.Atoi(msg.String())
		m.tab = n - 1
		return m, nil

	case "r":
		if err := m.reload(); err != nil {
			m.err = err
		} else {
			m.err = nil
			m.status = "Reloaded."
		}
		return m, nil

	case "up", "k":
		switch m.tab {
		case tabBlocks:
			if m.blockCur > 0 {
				m.blockCur--
			}
		case tabGoals:
			if m.goalCur > 0 {
				m.goalCur--
			}
		}
		return m, nil

	case "down", "j":
		switch m.tab {
		case tabBlocks:
			if m.blockCur < len(m.blocks)-1 {
				m.blockCur++
			}
		case tabGoals:
			if m.goalCur < len(m.goals)-1 {
				m.goalCur++
			}
		}
		return m, nil

	case "c":
		if m.tab == tabBlocks && len(m.blocks) > 0 {
			b := m.blocks[m.blockCur]
			if !b.closed {
				m.mode = "close"
				m.form = closeFormState{blockIdx: m.blockCur}
			} else {
				m.status = "That block is already closed."
			}
		}
		return m, nil

	case "n":
		if m.tab == tabBlocks {
			if len(m.projects) == 0 {
				m.status = "No projects yet — add one with 'journal project add' first."
				return m, nil
			}
			m.mode = "start_project"
			m.projectCur = 0
		}
		return m, nil

	case "enter", " ":
		if m.tab == tabGoals && len(m.goals) > 0 {
			g := &m.goals[m.goalCur]
			newDone := !g.done
			if _, err := conn.Exec(`UPDATE weekly_goals SET done = ? WHERE id = ?`, newDone, g.id); err != nil {
				m.err = err
			} else {
				g.done = newDone
				m.err = nil
			}
		}
		return m, nil
	}
	return m, nil
}

func (m tuiModel) updateCloseForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.form

	switch msg.String() {
	case "esc":
		m.mode = "browse"
		return m, nil

	case "ctrl+c":
		return m, tea.Quit

	case "backspace":
		if s := f.fields[f.field]; len(s) > 0 {
			f.fields[f.field] = s[:len(s)-1]
		}
		return m, nil

	case "tab", "down":
		f.field = (f.field + 1) % len(closeFieldLabels)
		return m, nil

	case "shift+tab", "up":
		f.field = (f.field - 1 + len(closeFieldLabels)) % len(closeFieldLabels)
		return m, nil

	case "enter":
		if f.field < len(closeFieldLabels)-1 {
			f.field++
			return m, nil
		}
		return m.submitCloseForm()
	}

	if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
		f.fields[f.field] += string(msg.Runes)
	}
	return m, nil
}

func (m tuiModel) submitCloseForm() (tea.Model, tea.Cmd) {
	f := m.form
	done := strings.TrimSpace(f.fields[0])
	notDone := strings.TrimSpace(f.fields[1])
	nextStep := strings.TrimSpace(f.fields[2])
	filesLinks := strings.TrimSpace(f.fields[3])
	focusRaw := strings.TrimSpace(f.fields[4])
	tweak := strings.TrimSpace(f.fields[5])

	if done == "" || notDone == "" || nextStep == "" {
		m.form.errMsg = "Done, Not done, and Next step can't be blank."
		return m, nil
	}
	focus, err := strconv.Atoi(focusRaw)
	if err != nil || focus < 1 || focus > 5 {
		m.form.errMsg = "Focus quality must be a number 1-5."
		return m, nil
	}

	b := m.blocks[f.blockIdx]

	_, err = conn.Exec(
		`UPDATE blocks
		 SET done_notes = ?, not_done_notes = ?, next_step = ?,
		     focus_quality = ?, tweak = ?,
		     closed_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		done, notDone, nextStep, focus, tweak, b.id,
	)
	if err == nil && filesLinks != "" {
		_, err = conn.Exec(
			`UPDATE blocks SET files_links = CASE
				WHEN files_links IS NULL OR files_links = '' THEN ?
				ELSE files_links || ' | ' || ?
			 END WHERE id = ?`,
			filesLinks, filesLinks, b.id,
		)
	}
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = fmt.Sprintf("Block #%d closed.", b.blockNum)
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) updateStartProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = "browse"
		return m, nil

	case "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.projectCur > 0 {
			m.projectCur--
		}
		return m, nil

	case "down", "j":
		if m.projectCur < len(m.projects)-1 {
			m.projectCur++
		}
		return m, nil

	case "enter":
		p := m.projects[m.projectCur]
		m.startForm = startFormState{projectID: p.id, projectName: p.name}
		m.mode = "start_fields"
		return m, nil
	}
	return m, nil
}

func (m tuiModel) updateStartFields(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.startForm

	switch msg.String() {
	case "esc":
		m.mode = "browse"
		return m, nil

	case "ctrl+c":
		return m, tea.Quit

	case "backspace":
		if s := f.fields[f.field]; len(s) > 0 {
			f.fields[f.field] = s[:len(s)-1]
		}
		return m, nil

	case "tab", "down":
		f.field = (f.field + 1) % len(startFieldLabels)
		return m, nil

	case "shift+tab", "up":
		f.field = (f.field - 1 + len(startFieldLabels)) % len(startFieldLabels)
		return m, nil

	case "enter":
		if f.field < len(startFieldLabels)-1 {
			f.field++
			return m, nil
		}
		return m.submitStartForm()
	}

	if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
		f.fields[f.field] += msg.String()
	}
	return m, nil
}

func (m tuiModel) submitStartForm() (tea.Model, tea.Cmd) {
	f := m.startForm
	outcome := strings.TrimSpace(f.fields[0])
	contextReload := strings.TrimSpace(f.fields[1])
	firstAction := strings.TrimSpace(f.fields[2])

	if outcome == "" || contextReload == "" || firstAction == "" {
		m.startForm.errMsg = "All three fields are required."
		return m, nil
	}

	today := time.Now().Format("2006-01-02")

	var nextNum int
	if err := conn.QueryRow(
		`SELECT COALESCE(MAX(block_num), 0) + 1 FROM blocks WHERE date = ?`, today,
	).Scan(&nextNum); err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	_, err := conn.Exec(
		`INSERT INTO blocks (date, block_num, day, project_id, outcome, context_reload, first_action)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		today, nextNum, time.Now().Format("Mon"), f.projectID, outcome, contextReload, firstAction,
	)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = fmt.Sprintf("Block #%d started.", nextNum)
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
		m.blockCur = 0 // newest block sorts first
	}
	return m, nil
}

// ---------- View ----------

func (m tuiModel) View() string {
	var b strings.Builder

	tabs := make([]string, tabCount)
	for i, name := range tabNames {
		if i == m.tab {
			tabs[i] = activeTabStyle.Render(name)
		} else {
			tabs[i] = inactiveTabStyle.Render(name)
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	b.WriteString("\n\n")

	switch m.mode {
	case "close":
		b.WriteString(m.viewCloseForm())
	case "start_project":
		b.WriteString(m.viewStartProject())
	case "start_fields":
		b.WriteString(m.viewStartFields())
	default:
		switch m.tab {
		case tabBlocks:
			b.WriteString(m.viewBlocks())
		case tabGoals:
			b.WriteString(m.viewGoals())
		case tabSleep:
			b.WriteString(m.viewSleep())
		case tabMetrics:
			b.WriteString(m.viewMetrics())
		}
	}

	b.WriteString("\n")
	if m.err != nil {
		b.WriteString(errStyle.Render("Error: " + m.err.Error()))
		b.WriteString("\n")
	}
	if m.status != "" {
		b.WriteString(okStyle.Render(m.status))
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render(m.helpLine()))

	return b.String()
}

func (m tuiModel) helpLine() string {
	switch m.mode {
	case "close", "start_fields":
		return "tab/↓: next field  •  shift+tab/↑: prev field  •  enter (last field): save  •  esc: cancel"
	case "start_project":
		return "↑↓/jk: move  •  enter: pick project  •  esc: cancel"
	}
	switch m.tab {
	case tabBlocks:
		return "tab: switch section  •  ↑↓/jk: move  •  n: new block  •  c: close selected open block  •  r: reload  •  q: quit"
	case tabGoals:
		return "tab: switch section  •  ↑↓/jk: move  •  enter/space: toggle done  •  r: reload  •  q: quit"
	default:
		return "tab: switch section  •  r: reload  •  q: quit"
	}
}

func (m tuiModel) viewBlocks() string {
	if len(m.blocks) == 0 {
		return headerStyle.Render("=== Blocks (this week) ===") + "\n" + dimStyle.Render("No blocks logged this week.")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("=== Blocks (this week) ==="))
	b.WriteString("\n")

	for i, blk := range m.blocks {
		status := "closed"
		if !blk.closed {
			status = "OPEN"
		}
		focus := "-"
		if blk.focus != nil {
			focus = strconv.Itoa(*blk.focus)
		}
		line := fmt.Sprintf("%s #%-2d [%-6s] %-10s focus:%-2s %s", blk.date, blk.blockNum, status, blk.project, focus, blk.outcome)
		if i == m.blockCur {
			b.WriteString(cursorStyle.Render("> " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m tuiModel) viewGoals() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("=== Weekly Goals ==="))
	b.WriteString("\n")

	if len(m.goals) == 0 {
		b.WriteString(dimStyle.Render("No goals yet — add one with 'journal goal add'."))
		return b.String()
	}

	for i, g := range m.goals {
		mark := " "
		if g.done {
			mark = "x"
		}
		line := fmt.Sprintf("[%s] %-4s %s", mark, g.day, g.goal)
		if i == m.goalCur {
			b.WriteString(cursorStyle.Render("> " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m tuiModel) viewSleep() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("=== Sleep (this week) ==="))
	b.WriteString("\n")

	if len(m.sleep) == 0 {
		b.WriteString(dimStyle.Render("No checkins logged this week."))
		return b.String()
	}

	var sumHours float64
	var sumQuality, sumFeel int
	for _, s := range m.sleep {
		b.WriteString(fmt.Sprintf("  %s  sleep:%.1fh  quality:%d  feel:%d\n", s.date, s.hours, s.quality, s.feel))
		sumHours += s.hours
		sumQuality += s.quality
		sumFeel += s.feel
	}
	n := float64(len(m.sleep))
	b.WriteString(fmt.Sprintf("\nAvg sleep: %.1fh | Avg quality: %.1f | Avg feel: %.1f\n",
		sumHours/n, float64(sumQuality)/n, float64(sumFeel)/n))
	return b.String()
}

func (m tuiModel) viewMetrics() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("=== Blocks by project (this week) ==="))
	b.WriteString("\n")

	if len(m.metrics) == 0 {
		b.WriteString(dimStyle.Render("No blocks logged this week."))
		return b.String()
	}

	for _, mt := range m.metrics {
		b.WriteString(fmt.Sprintf("  %-15s blocks:%-3d avg focus:%.1f\n", mt.project, mt.count, mt.avgFocus))
	}
	return b.String()
}

func (m tuiModel) viewCloseForm() string {
	if len(m.blocks) == 0 || m.form.blockIdx >= len(m.blocks) {
		return errStyle.Render("No block selected.")
	}
	blk := m.blocks[m.form.blockIdx]

	var b strings.Builder
	b.WriteString(bannerStyle.Render(fmt.Sprintf("Closing block #%d — %s", blk.blockNum, blk.outcome)))
	b.WriteString("\n")

	for i, label := range closeFieldLabels {
		marker := "  "
		if i == m.form.field {
			marker = cursorStyle.Render("> ")
		}
		b.WriteString(fmt.Sprintf("%s%-14s %s\n", marker, label+":", m.form.fields[i]))
	}
	if m.form.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render(m.form.errMsg))
		b.WriteString("\n")
	}
	return b.String()
}

func (m tuiModel) viewStartProject() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("=== New block: pick a project ==="))
	b.WriteString("\n")

	for i, p := range m.projects {
		if i == m.projectCur {
			b.WriteString(cursorStyle.Render("> " + p.name))
		} else {
			b.WriteString("  " + p.name)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m tuiModel) viewStartFields() string {
	f := m.startForm

	var b strings.Builder
	b.WriteString(bannerStyle.Render(fmt.Sprintf("New block — %s", f.projectName)))
	b.WriteString("\n")

	for i, label := range startFieldLabels {
		marker := "  "
		if i == f.field {
			marker = cursorStyle.Render("> ")
		}
		b.WriteString(fmt.Sprintf("%s%-16s %s\n", marker, label+":", f.fields[i]))
	}
	if f.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render(f.errMsg))
		b.WriteString("\n")
	}
	return b.String()
}

// ---------- command ----------

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the full-screen dashboard (blocks, goals, sleep, metrics)",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := newTUIModel()
		if err != nil {
			return err
		}
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
