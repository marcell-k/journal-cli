package cmd

import (
	"database/sql"
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
	tabProjects
	tabMetrics
	tabCount
)

var tabNames = [tabCount]string{"Blocks", "Goals", "Sleep", "Projects", "Metrics"}

// ---------- styles ----------

var (
	activeTabStyle   = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(lipgloss.Color("#e6c384")).Underline(true)
	inactiveTabStyle = lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("#a6a69c"))
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e6c384"))
	bannerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7fb4ca")).
				Border(lipgloss.RoundedBorder()).Padding(0, 1).MarginTop(1).MarginBottom(1)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6c384"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6a69c"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#e46876"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#87a987"))
)

// ---------- open-block header info ----------

type openBlockInfo struct {
	blockNum  int
	outcome   string
	project   string
	startedAt time.Time
}

var (
	headerBarStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c9c9c1")).MarginBottom(1)
	blockLenMins   = 90
)
var displayLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.Local
	}
	return loc
}()

type tickMsg time.Time

func tickEvery() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

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
type goalsDay struct {
	name  string // Mon..Sun
	date  string // YYYY-MM-DD
	goals []tuiGoal
}

type goalsWeek struct {
	weekStart string // Monday, YYYY-MM-DD
	weekEnd   string // Sunday, YYYY-MM-DD
	num       int    // 1-based, oldest tracked week = #1
	days      [7]goalsDay
}

var dayOrder = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

func todayDayIndex() int {
	wd := time.Now().Weekday()
	if wd == time.Sunday {
		return 6
	}
	return int(wd) - 1
}

type tuiSleep struct {
	date          string
	weekday       string
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

// ---------- generic form ----------
type form struct {
	title  string
	labels []string
	values []string
	field  int
	errMsg string

	ctxID    int    // e.g. block id, goal id, project id being edited
	ctxLabel string // e.g. the project's old name, for a friendly status message
}

func newForm(title string, labels []string, values []string) form {
	if values == nil {
		values = make([]string, len(labels))
	}
	return form{title: title, labels: labels, values: values}
}

func (f *form) handleKey(msg tea.KeyMsg) (submitted bool) {
	switch msg.String() {
	case "backspace":
		if s := f.values[f.field]; len(s) > 0 {
			f.values[f.field] = s[:len(s)-1]
		}
	case "tab", "down":
		f.field = (f.field + 1) % len(f.labels)
	case "shift+tab", "up":
		f.field = (f.field - 1 + len(f.labels)) % len(f.labels)
	case "enter":
		if f.field < len(f.labels)-1 {
			f.field++
		} else {
			return true
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			f.values[f.field] += string(msg.Runes)
		}
	}
	return false
}

var (
	closeFieldLabels         = []string{"Done", "Not done", "Next step", "Files/links", "Focus (1-10)", "Tweak"}
	blockUpdateFieldLabels   = []string{"Done notes", "Deliverable/checkpoint", "Files/links"}
	blockStartFieldLabels    = []string{"Outcome", "Context reload", "First action"}
	sleepFieldLabels         = []string{"Day (blank=today)", "Hours", "Quality (1-10)", "Feel (1-10)"}
	goalAddFieldLabels       = []string{"Goal", "Day (blank=today, or mon/tue/...)"}
	goalEditFieldLabels      = []string{"Goal"}
	projectAddFieldLabels    = []string{"Name"}
	projectRenameFieldLabels = []string{"New name"}
)

// ---------- delete confirmation ----------

type pendingDelete struct {
	kind    string // "block" | "goal" | "sleep" | "project"
	id      int    // for block / goal / project
	dateKey string // for sleep (daily_checkin is keyed by date, not id)
	label   string // human-readable description shown in the confirm prompt
}

// ---------- block detail (read-only) ----------

type tuiBlockDetail struct {
	id, blockNum int
	date, day    string

	project, outcome, contextReload, firstAction string

	deliverable, doneNotes, notDoneNotes string
	nextStep, filesLinks, tweak          string

	focus *int

	createdAt string
	closedAt  *string
}

// ---------- model ----------

type tuiModel struct {
	tab int

	blocks   []tuiBlock
	blockCur int

	goalWeeks        []goalsWeek
	goalWeekExpanded int
	goalRevealFrom   int
	goalCurWeek      int
	goalCurDay       int
	goalCurGoal      int

	sleep    []tuiSleep
	sleepCur int

	metrics []tuiMetric

	projects   []tuiProject
	projectCur int

	openBlock *openBlockInfo

	// mode: "browse" | "form" | "start_project" | "block_detail" | "confirm_delete"
	mode        string
	formPurpose string
	f           form

	pendingDelete pendingDelete
	blockDetail   tuiBlockDetail

	status string
	err    error

	width, height int
}

func newTUIModel() (tuiModel, error) {
	m := tuiModel{mode: "browse"}
	m.goalWeekExpanded = 0
	m.goalRevealFrom = todayDayIndex()
	m.goalCurWeek = 0
	m.goalCurDay = m.goalRevealFrom
	m.goalCurGoal = -1
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

	loadCount := len(m.goalWeeks)
	if loadCount == 0 {
		loadCount = 4
	}
	goalWeeks, err := loadGoalWeeks(weekStart, loadCount)
	if err != nil {
		return err
	}
	m.goalWeeks = goalWeeks
	if m.goalWeekExpanded >= len(m.goalWeeks) {
		m.goalWeekExpanded = 0
	}
	if m.goalCurWeek >= len(m.goalWeeks) {
		m.goalCurWeek = 0
		m.goalCurDay = m.goalRevealFrom
		m.goalCurGoal = -1
	} else if m.goalCurDay >= 0 {
		w := m.goalWeeks[m.goalCurWeek]
		if m.goalCurDay > 6 {
			m.goalCurDay = 6
		}
		if m.goalCurGoal >= len(w.days[m.goalCurDay].goals) {
			m.goalCurGoal = len(w.days[m.goalCurDay].goals) - 1
		}
	}

	sleep, err := loadTUISleep(weekStart)
	if err != nil {
		return err
	}
	m.sleep = sleep
	if m.sleepCur >= len(m.sleep) {
		m.sleepCur = len(m.sleep) - 1
	}
	if m.sleepCur < 0 {
		m.sleepCur = 0
	}

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
	openBlock, err := loadOpenBlock()
	if err != nil {
		return err
	}
	m.openBlock = openBlock

	return nil
}

func loadOpenBlock() (*openBlockInfo, error) {
	var ob openBlockInfo
	var project sql.NullString
	var createdAt string
	err := conn.QueryRow(`
		SELECT b.block_num, b.outcome, p.name, b.created_at
		FROM blocks b LEFT JOIN projects p ON p.id = b.project_id
		WHERE b.closed_at IS NULL
		ORDER BY b.id DESC LIMIT 1`,
	).Scan(&ob.blockNum, &ob.outcome, &project, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ob.project = "-"
	if project.Valid {
		ob.project = project.String
	}
	t, perr := time.ParseInLocation("2006-01-02 15:04:05", createdAt, time.UTC)
	if perr != nil {
		if t2, perr2 := time.Parse(time.RFC3339, createdAt); perr2 == nil {
			t = t2
		} else {
			t = time.Now().UTC()
		}
	}
	ob.startedAt = t.In(displayLoc)
	return &ob, nil
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

func weekDates(weekStart string) [7]string {
	var out [7]string
	t, _ := time.Parse("2006-01-02", weekStart)
	for i := 0; i < 7; i++ {
		out[i] = t.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

// firstWeekStart returns the earliest week_start ever logged, or "" if none.
func firstWeekStart() (string, error) {
	var s sql.NullString
	if err := conn.QueryRow(`SELECT MIN(week_start) FROM weekly_goals`).Scan(&s); err != nil {
		return "", err
	}
	if !s.Valid {
		return "", nil
	}
	return s.String, nil
}

// weekNumFor computes a 1-based, ever-increasing week number relative to firstStart.
func weekNumFor(weekStart, firstStart string) int {
	if firstStart == "" {
		return 1
	}
	t1, _ := time.Parse("2006-01-02", firstStart)
	t2, _ := time.Parse("2006-01-02", weekStart)
	days := t2.Sub(t1).Hours() / 24
	return int(days/7) + 1
}

func loadGoalWeek(weekStart string, num int) (goalsWeek, error) {
	dates := weekDates(weekStart)
	w := goalsWeek{weekStart: weekStart, weekEnd: dates[6], num: num}
	for i, name := range dayOrder {
		w.days[i] = goalsDay{name: name, date: dates[i]}
	}

	rows, err := conn.Query(
		`SELECT id, day, goal, done FROM weekly_goals WHERE week_start = ? ORDER BY id`,
		weekStart,
	)
	if err != nil {
		return w, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		n++
		var g tuiGoal
		if err := rows.Scan(&g.id, &g.day, &g.goal, &g.done); err != nil {
			return w, err
		}
		g.num = n
		for i, name := range dayOrder {
			if name == g.day {
				w.days[i].goals = append(w.days[i].goals, g)
				break
			}
		}
	}
	return w, rows.Err()
}

func loadGoalWeeks(currentWeekStart string, count int) ([]goalsWeek, error) {
	firstStart, err := firstWeekStart()

	if err != nil {
		return nil, err
	}
	var out []goalsWeek
	t, _ := time.Parse("2006-01-02", currentWeekStart)
	for i := 0; i < count; i++ {
		ws := t.AddDate(0, 0, -7*i).Format("2006-01-02")
		w, err := loadGoalWeek(ws, weekNumFor(ws, firstStart))
		if err != nil {
			return nil, err
		}
		out = append(out, w)
		if firstStart != "" && ws <= firstStart {
			break
		}
	}
	return out, nil
}
func (m *tuiModel) loadMoreGoalWeeks(count int) {
	if len(m.goalWeeks) == 0 {
		return
	}
	last := m.goalWeeks[len(m.goalWeeks)-1]
	firstStart, err := firstWeekStart()
	if err != nil {
		m.err = err
		return
	}
	if firstStart != "" && last.weekStart <= firstStart {
		return
	}
	t, _ := time.Parse("2006-01-02", last.weekStart)
	for i := 1; i <= count; i++ {
		ws := t.AddDate(0, 0, -7*i).Format("2006-01-02")
		w, err := loadGoalWeek(ws, weekNumFor(ws, firstStart))
		if err != nil {
			m.err = err
			return
		}
		m.goalWeeks = append(m.goalWeeks, w)
		if firstStart != "" && ws <= firstStart {
			break
		}
	}
}

func (m tuiModel) currentGoalWeek() *goalsWeek {
	if m.goalCurWeek < 0 || m.goalCurWeek >= len(m.goalWeeks) {
		return nil
	}
	return &m.goalWeeks[m.goalCurWeek]
}

func (m *tuiModel) goalUp() {
	if len(m.goalWeeks) == 0 {
		return
	}
	if m.goalCurGoal > 0 {
		m.goalCurGoal--
		return
	}
	if m.goalCurGoal == 0 {
		m.goalCurGoal = -1
		return
	}
	if m.goalCurDay >= 0 {
		if m.goalCurDay > 0 {
			m.goalCurDay--
			if m.goalCurWeek == 0 && m.goalCurDay < m.goalRevealFrom {
				m.goalRevealFrom = m.goalCurDay
			}
			m.goalCurGoal = -1
			return
		}
		m.goalCurDay = -1
		return
	}
	if m.goalCurWeek > 0 {
		prev := m.goalCurWeek - 1
		m.goalCurWeek = prev
		if prev == m.goalWeekExpanded {
			w := m.goalWeeks[prev]
			m.goalCurDay = 6
			if len(w.days[6].goals) > 0 {
				m.goalCurGoal = len(w.days[6].goals) - 1
			} else {
				m.goalCurGoal = -1
			}
		}
	}
}

func (m *tuiModel) goalDown() {
	if len(m.goalWeeks) == 0 {
		return
	}
	if m.goalCurDay >= 0 {
		w := m.goalWeeks[m.goalCurWeek]
		day := w.days[m.goalCurDay]
		if m.goalCurGoal < len(day.goals)-1 {
			m.goalCurGoal++
			return
		}
		if m.goalCurDay < 6 {
			m.goalCurDay++
			m.goalCurGoal = -1
			return
		}
		m.goalAdvanceWeek()
		return
	}
	if m.goalCurWeek == m.goalWeekExpanded {
		start := 0
		if m.goalCurWeek == 0 {
			start = m.goalRevealFrom
		}
		m.goalCurDay = start
		m.goalCurGoal = -1
		return
	}
	m.goalAdvanceWeek()
}

func (m *tuiModel) goalAdvanceWeek() {
	if m.goalCurWeek >= len(m.goalWeeks)-1 {
		m.loadMoreGoalWeeks(4)
	}
	if m.goalCurWeek < len(m.goalWeeks)-1 {
		m.goalCurWeek++
		m.goalCurDay = -1
		m.goalCurGoal = -1
	}
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
		var rawDate string
		if err := rows.Scan(&rawDate, &s.hours, &s.quality, &s.feel); err != nil {
			return nil, err
		}
		day := rawDate
		if t, err := time.Parse(time.RFC3339, rawDate); err == nil {
			day = t.Format("2006-01-02")
			s.weekday = t.Format("Mon")
		} else if t, err := time.Parse("2006-01-02", rawDate); err == nil {
			s.weekday = t.Format("Mon")
		}
		s.date = day
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

func loadBlockDetail(id int) (tuiBlockDetail, error) {
	var d tuiBlockDetail
	var project sql.NullString
	var deliverable, doneNotes, notDoneNotes, nextStep, filesLinks, tweak sql.NullString
	var focus sql.NullInt64
	var closedAt sql.NullString

	err := conn.QueryRow(`
		SELECT b.id, b.date, b.block_num, b.day, p.name, b.outcome, b.context_reload, b.first_action,
		       b.deliverable, b.done_notes, b.not_done_notes, b.next_step, b.files_links,
		       b.focus_quality, b.tweak, b.created_at, b.closed_at
		FROM blocks b LEFT JOIN projects p ON p.id = b.project_id
		WHERE b.id = ?`, id,
	).Scan(&d.id, &d.date, &d.blockNum, &d.day, &project, &d.outcome, &d.contextReload, &d.firstAction,
		&deliverable, &doneNotes, &notDoneNotes, &nextStep, &filesLinks,
		&focus, &tweak, &d.createdAt, &closedAt)
	if err != nil {
		return d, err
	}

	d.project = "-"
	if project.Valid {
		d.project = project.String
	}
	d.deliverable = nullOr(deliverable)
	d.doneNotes = nullOr(doneNotes)
	d.notDoneNotes = nullOr(notDoneNotes)
	d.nextStep = nullOr(nextStep)
	d.filesLinks = nullOr(filesLinks)
	d.tweak = nullOr(tweak)
	if focus.Valid {
		f := int(focus.Int64)
		d.focus = &f
	}
	if closedAt.Valid {
		c := closedAt.String
		d.closedAt = &c
	}
	return d, nil
}

func nullOr(v sql.NullString) string {
	if v.Valid && v.String != "" {
		return v.String
	}
	return "-"
}

// ---------- tea.Model ----------

func (m tuiModel) Init() tea.Cmd { return tickEvery() }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		// No-op besides re-scheduling: View() reads time.Now() directly,
		// so simply causing a re-render every 5s keeps the header live.
		return m, tickEvery()

	case tea.KeyMsg:
		m.status = ""
		switch m.mode {
		case "form":
			return m.updateFormMode(msg)
		case "start_project":
			return m.updateStartProject(msg)
		case "block_detail":
			return m.updateBlockDetail(msg)
		case "confirm_delete":
			return m.updateConfirmDelete(msg)
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
	case "1", "2", "3", "4", "5":
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
			m.goalUp()
		case tabSleep:
			if m.sleepCur > 0 {
				m.sleepCur--
			}
		case tabProjects:
			if m.projectCur > 0 {
				m.projectCur--
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
			m.goalDown()
		case tabSleep:
			if m.sleepCur < len(m.sleep)-1 {
				m.sleepCur++
			}
		case tabProjects:
			if m.projectCur < len(m.projects)-1 {
				m.projectCur++
			}
		}
		return m, nil

	// n: create — same key, same meaning, on every tab.
	case "n":
		switch m.tab {
		case tabBlocks:
			if len(m.projects) == 0 {
				m.status = "No projects yet — switch to the Projects tab and press n to add one."
				return m, nil
			}
			m.mode = "start_project"
			m.projectCur = 0
		case tabGoals:
			w := m.currentGoalWeek()
			if w == nil {
				return m, nil
			}
			dayPrefill := ""
			if m.goalCurDay >= 0 {
				dayPrefill = w.days[m.goalCurDay].name
			}
			m.mode = "form"
			m.formPurpose = "goal_add"
			m.f = newForm("New goal", goalAddFieldLabels, []string{"", dayPrefill})
			m.f.ctxLabel = w.weekStart
		case tabSleep:
			m.mode = "form"
			m.formPurpose = "sleep_save"
			m.f = newForm("Add sleep checkin", sleepFieldLabels, nil)
		case tabProjects:
			m.mode = "form"
			m.formPurpose = "project_add"
			m.f = newForm("New project", projectAddFieldLabels, nil)
		}
		return m, nil

	// u: update — same key, same meaning, on every tab.
	case "u":
		switch m.tab {
		case tabBlocks:
			if len(m.blocks) == 0 {
				return m, nil
			}
			b := m.blocks[m.blockCur]
			if b.closed {
				m.status = "That block is already closed."
				return m, nil
			}
			m.mode = "form"
			m.formPurpose = "block_update"
			m.f = newForm(fmt.Sprintf("Updating block #%d — %s", b.blockNum, b.outcome), blockUpdateFieldLabels, nil)
			m.f.ctxID = b.id
		case tabGoals:
			w := m.currentGoalWeek()
			if w == nil || m.goalCurDay < 0 || m.goalCurGoal < 0 {
				return m, nil
			}
			g := w.days[m.goalCurDay].goals[m.goalCurGoal]
			m.mode = "form"
			m.formPurpose = "goal_edit"
			m.f = newForm(fmt.Sprintf("Editing goal #%d", g.num), goalEditFieldLabels, []string{g.goal})
			m.f.ctxID = g.id
		case tabSleep:
			if len(m.sleep) == 0 {
				m.status = "No sleep checkins this week yet — press n to add one."
				return m, nil
			}
			s := m.sleep[m.sleepCur]
			m.mode = "form"
			m.formPurpose = "sleep_save"
			m.f = newForm("Update sleep checkin", sleepFieldLabels, []string{
				s.date,
				strconv.FormatFloat(s.hours, 'f', -1, 64),
				strconv.Itoa(s.quality),
				strconv.Itoa(s.feel),
			})
		case tabProjects:
			if len(m.projects) == 0 {
				return m, nil
			}
			p := m.projects[m.projectCur]
			m.mode = "form"
			m.formPurpose = "project_rename"
			m.f = newForm(fmt.Sprintf("Renaming project %q", p.name), projectRenameFieldLabels, []string{p.name})
			m.f.ctxID = p.id
			m.f.ctxLabel = p.name
		}
		return m, nil

	// d: delete — same key, same meaning, on every tab. Always confirms first.
	case "d":
		switch m.tab {
		case tabBlocks:
			if len(m.blocks) == 0 {
				return m, nil
			}
			b := m.blocks[m.blockCur]
			m.mode = "confirm_delete"
			m.pendingDelete = pendingDelete{kind: "block", id: b.id, label: fmt.Sprintf("block #%d (%s)", b.blockNum, b.outcome)}
		case tabGoals:
			w := m.currentGoalWeek()
			if w == nil || m.goalCurDay < 0 || m.goalCurGoal < 0 {
				return m, nil
			}
			g := w.days[m.goalCurDay].goals[m.goalCurGoal]
			m.mode = "confirm_delete"
			m.pendingDelete = pendingDelete{kind: "goal", id: g.id, label: fmt.Sprintf("goal #%d (%s)", g.num, g.goal)}
		case tabSleep:
			if len(m.sleep) == 0 {
				return m, nil
			}
			s := m.sleep[m.sleepCur]
			m.mode = "confirm_delete"
			m.pendingDelete = pendingDelete{kind: "sleep", dateKey: s.date, label: fmt.Sprintf("sleep checkin for %s", s.date)}
		case tabProjects:
			if len(m.projects) == 0 {
				return m, nil
			}
			p := m.projects[m.projectCur]
			var count int
			if err := conn.QueryRow(`SELECT COUNT(*) FROM blocks WHERE project_id = ?`, p.id).Scan(&count); err != nil {
				m.err = err
				return m, nil
			}
			if count > 0 {
				m.status = fmt.Sprintf("Project %q has %d block(s) logged against it — rename it instead, or reassign those blocks first.", p.name, count)
				return m, nil
			}
			m.mode = "confirm_delete"
			m.pendingDelete = pendingDelete{kind: "project", id: p.id, label: fmt.Sprintf("project %q", p.name)}
		}
		return m, nil

	// c: close — distinct from "update"; only applies to an open block.
	case "c":
		if m.tab == tabBlocks && len(m.blocks) > 0 {
			b := m.blocks[m.blockCur]
			if b.closed {
				m.status = "That block is already closed."
				return m, nil
			}
			m.mode = "form"
			m.formPurpose = "block_close"
			m.f = newForm(fmt.Sprintf("Closing block #%d — %s", b.blockNum, b.outcome), closeFieldLabels, nil)
			m.f.ctxID = b.id
		}
		return m, nil

	case "enter", " ":
		switch m.tab {
		case tabGoals:
			if m.goalCurDay == -1 {
				if msg.String() == "enter" {
					m.goalWeekExpanded = m.goalCurWeek
				}
				return m, nil
			}
			w := m.currentGoalWeek()
			if w == nil || m.goalCurGoal < 0 {
				return m, nil
			}
			g := &w.days[m.goalCurDay].goals[m.goalCurGoal]
			newDone := !g.done
			if _, err := conn.Exec(`UPDATE weekly_goals SET done = ? WHERE id = ?`, newDone, g.id); err != nil {
				m.err = err
			} else {
				g.done = newDone
				m.err = nil
			}
		case tabBlocks:
			if msg.String() == "enter" && len(m.blocks) > 0 {
				blk := m.blocks[m.blockCur]
				detail, err := loadBlockDetail(blk.id)
				if err != nil {
					m.err = err
				} else {
					m.err = nil
					m.blockDetail = detail
					m.mode = "block_detail"
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// updateFormMode handles the shared "form" mode; navigation/typing is
// delegated to form.handleKey, submission is dispatched by formPurpose.
func (m tuiModel) updateFormMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = "browse"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}

	if m.f.handleKey(msg) {
		return m.submitForm()
	}
	return m, nil
}

func (m tuiModel) submitForm() (tea.Model, tea.Cmd) {
	switch m.formPurpose {
	case "block_start":
		return m.submitBlockStart()
	case "block_update":
		return m.submitBlockUpdate()
	case "block_close":
		return m.submitBlockClose()
	case "goal_add":
		return m.submitGoalAdd()
	case "goal_edit":
		return m.submitGoalEdit()
	case "sleep_save":
		return m.submitSleepSave()
	case "project_add":
		return m.submitProjectAdd()
	case "project_rename":
		return m.submitProjectRename()
	}
	m.mode = "browse"
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
		m.mode = "form"
		m.formPurpose = "block_start"
		m.f = newForm(fmt.Sprintf("New block — %s", p.name), blockStartFieldLabels, nil)
		m.f.ctxID = p.id
		return m, nil
	}
	return m, nil
}

func (m tuiModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		return m.executeDelete()
	case "n", "esc", "ctrl+c":
		m.mode = "browse"
		m.status = "Delete cancelled."
		return m, nil
	}
	return m, nil
}

func (m tuiModel) updateBlockDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "enter", "q":
		m.mode = "browse"
		return m, nil
	}
	return m, nil
}

// ---------- submit handlers ----------

func (m tuiModel) submitBlockStart() (tea.Model, tea.Cmd) {
	outcome := strings.TrimSpace(m.f.values[0])
	contextReload := strings.TrimSpace(m.f.values[1])
	firstAction := strings.TrimSpace(m.f.values[2])
	if outcome == "" || contextReload == "" || firstAction == "" {
		m.f.errMsg = "All three fields are required."
		return m, nil
	}

	today := time.Now().Format("2006-01-02")
	var nextNum int
	if err := conn.QueryRow(`SELECT COALESCE(MAX(block_num), 0) + 1 FROM blocks WHERE date = ?`, today).Scan(&nextNum); err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	_, err := conn.Exec(
		`INSERT INTO blocks (date, block_num, day, project_id, outcome, context_reload, first_action)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		today, nextNum, time.Now().Format("Mon"), m.f.ctxID, outcome, contextReload, firstAction,
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

func (m tuiModel) submitBlockUpdate() (tea.Model, tea.Cmd) {
	doneNotes := strings.TrimSpace(m.f.values[0])
	deliverable := strings.TrimSpace(m.f.values[1])
	filesLinks := strings.TrimSpace(m.f.values[2])

	if doneNotes == "" && deliverable == "" && filesLinks == "" {
		m.mode = "browse"
		m.status = "Nothing entered, no changes made."
		return m, nil
	}

	id := m.f.ctxID
	var err error
	if doneNotes != "" {
		_, err = conn.Exec(
			`UPDATE blocks SET done_notes = CASE
				WHEN done_notes IS NULL OR done_notes = '' THEN ?
				ELSE done_notes || ' | ' || ?
			 END WHERE id = ?`,
			doneNotes, doneNotes, id,
		)
	}
	if err == nil && deliverable != "" {
		_, err = conn.Exec(
			`UPDATE blocks SET deliverable = CASE
				WHEN deliverable IS NULL OR deliverable = '' THEN ?
				ELSE deliverable || ' | ' || ?
			 END WHERE id = ?`,
			deliverable, deliverable, id,
		)
	}
	if err == nil && filesLinks != "" {
		_, err = conn.Exec(
			`UPDATE blocks SET files_links = CASE
				WHEN files_links IS NULL OR files_links = '' THEN ?
				ELSE files_links || ' | ' || ?
			 END WHERE id = ?`,
			filesLinks, filesLinks, id,
		)
	}
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = "Block updated."
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) submitBlockClose() (tea.Model, tea.Cmd) {
	done := strings.TrimSpace(m.f.values[0])
	notDone := strings.TrimSpace(m.f.values[1])
	nextStep := strings.TrimSpace(m.f.values[2])
	filesLinks := strings.TrimSpace(m.f.values[3])
	focusRaw := strings.TrimSpace(m.f.values[4])
	tweak := strings.TrimSpace(m.f.values[5])

	focus, err := strconv.Atoi(focusRaw)
	if err != nil || focus < 1 || focus > 10 {
		m.f.errMsg = "Focus quality must be a number 1-10."
		return m, nil
	}

	id := m.f.ctxID
	_, err = conn.Exec(
		`UPDATE blocks
		 SET done_notes = ?, not_done_notes = ?, next_step = ?,
		     focus_quality = ?, tweak = ?,
		     closed_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		done, notDone, nextStep, focus, tweak, id,
	)
	if err == nil && filesLinks != "" {
		_, err = conn.Exec(
			`UPDATE blocks SET files_links = CASE
				WHEN files_links IS NULL OR files_links = '' THEN ?
				ELSE files_links || ' | ' || ?
			 END WHERE id = ?`,
			filesLinks, filesLinks, id,
		)
	}
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = "Block closed."
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) submitGoalAdd() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.f.values[0])
	dayRaw := strings.TrimSpace(m.f.values[1])

	if text == "" {
		m.f.errMsg = "Goal text can't be blank."
		return m, nil
	}

	weekStart := m.f.ctxLabel
	if weekStart == "" {
		weekStart = mondayOf(time.Now()).Format("2006-01-02")
	}

	day := "Mon"
	if weekStart == mondayOf(time.Now()).Format("2006-01-02") {
		day = time.Now().Format("Mon")
	}
	if dayRaw != "" {
		canonical, ok := validDays[strings.ToLower(dayRaw)]
		if !ok {
			m.f.errMsg = "Day must be one of: mon tue wed thu fri sat sun."
			return m, nil
		}
		day = canonical
	}

	_, err := conn.Exec(
		`INSERT INTO weekly_goals (week_start, day, goal) VALUES (?, ?, ?)`,
		weekStart, day, text,
	)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = fmt.Sprintf("Goal added for %s.", day)
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) submitGoalEdit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.f.values[0])
	if text == "" {
		m.f.errMsg = "Goal text can't be blank."
		return m, nil
	}

	_, err := conn.Exec(`UPDATE weekly_goals SET goal = ? WHERE id = ?`, text, m.f.ctxID)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = "Goal updated."
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) submitSleepSave() (tea.Model, tea.Cmd) {
	dayRaw := strings.TrimSpace(m.f.values[0])
	hoursRaw := strings.TrimSpace(m.f.values[1])
	qualityRaw := strings.TrimSpace(m.f.values[2])
	feelRaw := strings.TrimSpace(m.f.values[3])

	day := time.Now().Format("2006-01-02")
	if dayRaw != "" {
		if _, err := time.Parse("2006-01-02", dayRaw); err != nil {
			m.f.errMsg = "Day must be YYYY-MM-DD."
			return m, nil
		}
		day = dayRaw
	}

	hours, err := strconv.ParseFloat(hoursRaw, 64)
	if err != nil || hours < 0 || hours > 24 {
		m.f.errMsg = "Hours must be a number 0-24."
		return m, nil
	}
	quality, err := strconv.Atoi(qualityRaw)
	if err != nil || quality < 1 || quality > 10 {
		m.f.errMsg = "Quality must be a number 1-10."
		return m, nil
	}
	feel, err := strconv.Atoi(feelRaw)
	if err != nil || feel < 1 || feel > 10 {
		m.f.errMsg = "Feel must be a number 1-10."
		return m, nil
	}

	_, err = conn.Exec(
		`INSERT INTO daily_checkin (date, sleep_hours, sleep_quality, feel, notes)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(date) DO UPDATE SET
			sleep_hours = excluded.sleep_hours,
			sleep_quality = excluded.sleep_quality,
			feel = excluded.feel`,
		day, hours, quality, feel, "",
	)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = fmt.Sprintf("Checkin saved for %s.", day)
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) submitProjectAdd() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.f.values[0])
	if name == "" {
		m.f.errMsg = "Project name can't be blank."
		return m, nil
	}

	res, err := conn.Exec(`INSERT OR IGNORE INTO projects (name) VALUES (?)`, name)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	rows, _ := res.RowsAffected()
	m.mode = "browse"
	if rows == 0 {
		m.status = fmt.Sprintf("Project %q already exists.", name)
	} else {
		m.status = fmt.Sprintf("Project %q added.", name)
	}
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) submitProjectRename() (tea.Model, tea.Cmd) {
	newName := strings.TrimSpace(m.f.values[0])
	if newName == "" {
		m.f.errMsg = "Project name can't be blank."
		return m, nil
	}

	_, err := conn.Exec(`UPDATE projects SET name = ? WHERE id = ?`, newName, m.f.ctxID)
	if err != nil {
		m.f.errMsg = fmt.Sprintf("rename failed (name %q may already exist)", newName)
		return m, nil
	}

	m.mode = "browse"
	m.status = fmt.Sprintf("Renamed project %q to %q.", m.f.ctxLabel, newName)
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) executeDelete() (tea.Model, tea.Cmd) {
	pd := m.pendingDelete
	var err error
	switch pd.kind {
	case "block":
		_, err = conn.Exec(`DELETE FROM blocks WHERE id = ?`, pd.id)
	case "goal":
		_, err = conn.Exec(`DELETE FROM weekly_goals WHERE id = ?`, pd.id)
	case "sleep":
		_, err = conn.Exec(`DELETE FROM daily_checkin WHERE date = ?`, pd.dateKey)
	case "project":
		_, err = conn.Exec(`DELETE FROM projects WHERE id = ?`, pd.id)
	}

	m.mode = "browse"
	if err != nil {
		m.err = err
		return m, nil
	}

	m.status = "Deleted " + pd.label + "."
	m.err = nil
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	}
	return m, nil
}

// ---------- View ----------

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(m.headerBar())
	b.WriteString("\n")

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
	case "form":
		b.WriteString(m.viewForm())
	case "start_project":
		b.WriteString(m.viewStartProject())
	case "block_detail":
		b.WriteString(m.viewBlockDetail())
	case "confirm_delete":
		b.WriteString(m.viewConfirmDelete())
	default:
		switch m.tab {
		case tabBlocks:
			b.WriteString(m.viewBlocks())
		case tabGoals:
			b.WriteString(m.viewGoals())
		case tabSleep:
			b.WriteString(m.viewSleep())
		case tabProjects:
			b.WriteString(m.viewProjects())
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
func (m tuiModel) headerBar() string {
	now := time.Now().In(displayLoc)

	if m.openBlock == nil {
		return dimStyle.Render("○ No open block — press n on the Blocks tab to start one")
	}

	ob := m.openBlock
	elapsed := now.Sub(ob.startedAt)
	elapsed = max(elapsed, 0)
	endsAt := ob.startedAt.Add(time.Duration(blockLenMins) * time.Minute)
	remaining := endsAt.Sub(now)

	elapsedStr := fmt.Sprintf("%02d:%02d", int(elapsed.Minutes()), int(elapsed.Seconds())%60)

	timerStyle := okStyle
	statusWord := fmt.Sprintf("ends %s", endsAt.Format("15:04"))
	if remaining < 0 {
		timerStyle = errStyle
		statusWord = fmt.Sprintf("over by %02d:%02d", int(-remaining.Minutes()), int(-remaining.Seconds())%60)
	}

	status := timerStyle.Render(fmt.Sprintf("● Block #%d open (%s) — %s elapsed, %s — %s",
		ob.blockNum, ob.project, elapsedStr, statusWord, ob.outcome))

	return status
}

func (m tuiModel) helpLine() string {
	switch m.mode {
	case "form":
		return "tab/↓: next field  •  shift+tab/↑: prev field  •  enter (last field): save  •  esc: cancel"
	case "confirm_delete":
		return "y/enter: confirm delete  •  n/esc: cancel"
	case "start_project":
		return "↑↓/jk: move  •  enter: pick project  •  esc: cancel"
	case "block_detail":
		return "esc/enter/q: back to list"
	}
	switch m.tab {
	case tabBlocks:
		return "tab: switch section  •  ↑↓/jk: move  •  enter: view block  •  n: new  •  u: update  •  c: close  •  d: delete  •  r: reload  •  q: quit"
	case tabGoals:
		return "tab: switch section  •  ↑↓/jk: move  •  n: new  •  u: update  •  d: delete  •  enter/space: toggle done  •  r: reload  •  q: quit"
	case tabSleep:
		return "tab: switch section  •  ↑↓/jk: move  •  n: new  •  u: update  •  d: delete  •  r: reload  •  q: quit"
	case tabProjects:
		return "tab: switch section  •  ↑↓/jk: move  •  n: new  •  u: rename  •  d: delete  •  r: reload  •  q: quit"
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
			b.WriteString("  ")
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m tuiModel) viewGoals() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("=== Weekly Goals ==="))
	b.WriteString("\n")

	if len(m.goalWeeks) == 0 {
		b.WriteString(dimStyle.Render("No goals yet — press n to add one."))
		return b.String()
	}

	todayIdx := todayDayIndex()

	for wi, w := range m.goalWeeks {
		arrow := "▸"
		if wi == m.goalWeekExpanded {
			arrow = "▾"
		}
		marker := "  "
		if wi == m.goalCurWeek && m.goalCurDay == -1 {
			marker = cursorStyle.Render("> ")
		}
		b.WriteString(fmt.Sprintf("%s%s Week %s – %s  #%d\n", marker, arrow, w.weekStart, w.weekEnd, w.num))

		if wi != m.goalWeekExpanded {
			continue
		}

		start := 0
		if wi == 0 {
			start = m.goalRevealFrom
		}
		for di := start; di < 7; di++ {
			day := w.days[di]
			dayMarker := "    "
			if wi == m.goalCurWeek && m.goalCurDay == di && m.goalCurGoal == -1 {
				dayMarker = cursorStyle.Render("  > ")
			}
			label := day.name
			if wi == 0 && di == todayIdx {
				label = okStyle.Render(day.name + " (today)")
			}
			b.WriteString(dayMarker + label + "\n")

			if len(day.goals) == 0 {
				b.WriteString("        " + dimStyle.Render("(no goals)") + "\n")
				continue
			}
			for gi, g := range day.goals {
				mark := " "
				if g.done {
					mark = "x"
				}
				goalMarker := "      "
				if wi == m.goalCurWeek && m.goalCurDay == di && m.goalCurGoal == gi {
					goalMarker = cursorStyle.Render("    > ")
				}
				b.WriteString(fmt.Sprintf("%s[%s] %s\n", goalMarker, mark, g.goal))
			}
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
		b.WriteString(dimStyle.Render("No checkins logged this week — press n to add one."))
		return b.String()
	}

	var sumHours float64
	var sumQuality, sumFeel int
	for i, s := range m.sleep {
		line := fmt.Sprintf("%s %-3s  sleep:%.1fh  quality:%d  feel:%d", s.date, s.weekday, s.hours, s.quality, s.feel)
		if i == m.sleepCur {
			b.WriteString(cursorStyle.Render("> " + line))
		} else {
			b.WriteString("  ")
			b.WriteString(line)
		}
		b.WriteString("\n")
		sumHours += s.hours
		sumQuality += s.quality
		sumFeel += s.feel
	}
	n := float64(len(m.sleep))
	b.WriteString(fmt.Sprintf("\nAvg sleep: %.1fh | Avg quality: %.1f | Avg feel: %.1f\n",
		sumHours/n, float64(sumQuality)/n, float64(sumFeel)/n))
	return b.String()
}

func (m tuiModel) viewProjects() string {
	if len(m.projects) == 0 {
		return headerStyle.Render("=== Projects ===") + "\n" + dimStyle.Render("No projects yet — press n to add one.")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("=== Projects ==="))
	b.WriteString("\n")

	for i, p := range m.projects {
		line := fmt.Sprintf("%d) %s", p.id, p.name)
		if i == m.projectCur {
			b.WriteString(cursorStyle.Render("> " + line))
		} else {
			b.WriteString("  ")
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
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

// viewForm renders whatever form is currently active (new/update block,
// close block, add/edit goal, add/update sleep checkin, add/rename project).
func (m tuiModel) viewForm() string {
	var b strings.Builder
	b.WriteString(bannerStyle.Render(m.f.title))
	b.WriteString("\n")

	labelWidth := 0
	for _, l := range m.f.labels {
		if len(l) > labelWidth {
			labelWidth = len(l)
		}
	}

	for i, label := range m.f.labels {
		marker := "  "
		if i == m.f.field {
			marker = cursorStyle.Render("> ")
		}
		b.WriteString(fmt.Sprintf("%s%-*s %s\n", marker, labelWidth+1, label+":", m.f.values[i]))
	}
	if m.f.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render(m.f.errMsg))
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
			b.WriteString("  ")
			b.WriteString(p.name)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m tuiModel) viewConfirmDelete() string {
	var b strings.Builder
	b.WriteString(bannerStyle.Render("Confirm delete"))
	b.WriteString("\n")
	b.WriteString(errStyle.Render(fmt.Sprintf("Delete %s? This cannot be undone.", m.pendingDelete.label)))
	b.WriteString("\n")
	return b.String()
}

func (m tuiModel) viewBlockDetail() string {
	d := m.blockDetail

	var b strings.Builder
	b.WriteString(bannerStyle.Render(fmt.Sprintf("Block #%d (id=%d) — %s (%s)", d.blockNum, d.id, d.date, d.day)))
	b.WriteString("\n")

	row := func(label, val string) {
		b.WriteString(fmt.Sprintf("%-16s %s\n", label+":", val))
	}

	row("Project", d.project)
	row("Outcome", d.outcome)
	row("Context reload", d.contextReload)
	row("First action", d.firstAction)
	row("Deliverable", d.deliverable)
	row("Done", d.doneNotes)
	row("Not done", d.notDoneNotes)
	row("Next step", d.nextStep)
	row("Files/links", d.filesLinks)
	focus := "-"
	if d.focus != nil {
		focus = strconv.Itoa(*d.focus)
	}
	row("Focus quality", focus)
	row("Tweak", d.tweak)
	status := "open"
	if d.closedAt != nil {
		status = "closed at " + *d.closedAt
	}
	row("Status", status)
	row("Created", d.createdAt)

	return b.String()
}

// ---------- command ----------

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the full-screen dashboard (blocks, goals, sleep, projects, metrics)",
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
