package cmd

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/guptarohit/asciigraph"
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

var tabNames = [tabCount]string{"Blocks", "Goals", "Wellness", "Projects", "Metrics"}

// ---------- styles ----------

var (
	activeTabStyle   = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(lipgloss.Color("#e6c384")).Underline(true)
	inactiveTabStyle = lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("#a6a69c"))
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e6c384"))
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e6c384")).Padding(0, 1)
	tableCellStyle   = lipgloss.NewStyle().Padding(0, 1)
	tableBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4a4a45"))
	rowSelectedStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("#e6c384"))
	focusHighStyle   = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#87a987"))
	focusMidStyle    = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#e6c384"))
	focusLowStyle    = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#e46876"))

	bannerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7fb4ca")).
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
	length       string
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
	water         float64
	hasWater      bool
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
	case "alt+backspace":
		f.values[f.field] = deleteLastWord(f.values[f.field])
	case "shift+tab", "up":
		f.field = (f.field - 1 + len(f.labels)) % len(f.labels)
	case "ctrl+j":
		f.values[f.field] += "\n"
	case "enter":
		return true
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			f.values[f.field] += string(msg.Runes)
		}
	}
	return false
}
func deleteLastWord(s string) string {
	s = strings.TrimRight(s, " ")
	if i := strings.LastIndexByte(s, ' '); i >= 0 {
		return s[:i+1]
	}
	return ""
}

var (
	closeFieldLabels       = []string{"Done", "Not done", "Next step", "Files/links", "Focus (1-10)", "Tweak"}
	appendNoteFieldLabels  = []string{"Note"}
	blockUpdateFieldLabels = []string{
		"Outcome", "Context reload",
		"Deliverable", "Done", "Not done",
		"Files/links",
	}
	blockStartFieldLabels    = []string{"Outcome", "Context reload"}
	sleepFieldLabels         = []string{"Hours", "Quality (1-10)", "Feel (1-10)", "Water (L)", "Day"}
	goalAddFieldLabels       = []string{"Goal", "Day"}
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

	project, outcome, contextReload string

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

	metrics                   []tuiMetric
	correlation               tuiCorrelation
	weeklyTrend               tuiWeeklyTrend
	blockPosStats             []tuiBlockPosStat
	weekdayBest, weekdayWorst tuiWeekdayStat
	focusHist                 [5]int
	startTimeStats            []tuiStartTimeStat
	focusMedian               float64
	focusMean                 float64
	focusN                    int

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
	m.goalWeekExpanded = 0 // 0
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
	weekStart := mondayOf(time.Now().In(displayLoc)).Format("2006-01-02")

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

	correlation, err := loadTUICorrelation()
	if err != nil {
		return err
	}
	m.correlation = correlation

	weeklyTrend, err := loadWeeklyTrend(weekStart)
	if err != nil {
		return err
	}
	m.weeklyTrend = weeklyTrend

	blockPosStats, err := loadBlockPosStats()
	if err != nil {
		return err
	}
	m.blockPosStats = blockPosStats

	best, worst, err := loadWeekdayExtremes()
	if err != nil {
		return err
	}
	m.weekdayBest, m.weekdayWorst = best, worst
	startTimeStats, err := loadStartTimeStats()
	if err != nil {
		return err
	}
	m.startTimeStats = startTimeStats

	hist, median, mean, n, err := loadFocusDistribution()
	if err != nil {
		return err
	}
	m.focusHist, m.focusMedian, m.focusMean, m.focusN = hist, median, mean, n

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
		SELECT b.id, b.date, b.block_num, b.day, p.name, b.outcome, b.focus_quality, b.created_at, b.closed_at
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
		var createdAt string
		var closedAt *string
		if err := rows.Scan(&b.id, &b.date, &b.blockNum, &b.day, &project, &b.outcome, &focus, &createdAt, &closedAt); err != nil {
			return nil, err
		}
		if project != nil {
			b.project = *project
		} else {
			b.project = "-"
		}
		b.focus = focus
		b.closed = closedAt != nil
		b.length = "-"
		if ct, err := parseTimestamp(createdAt); err == nil {
			if closedAt != nil {
				if ct2, err2 := parseTimestamp(*closedAt); err2 == nil {
					b.length = formatDuration(ct2.Sub(ct))
				}
			} else {
				b.length = formatDuration(time.Now().Sub(ct))
			}
		}
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

type tuiCorrelation struct {
	pairedDays              int
	rHours, rQuality, rFeel float64
}

func loadTUICorrelation() (tuiCorrelation, error) {
	rows, err := conn.Query(`
		SELECT c.sleep_hours, c.sleep_quality, c.feel, AVG(b.focus_quality)
		FROM daily_checkin c
		JOIN blocks b ON b.date = c.date AND b.focus_quality IS NOT NULL
		GROUP BY c.date
		ORDER BY c.date`)
	if err != nil {
		return tuiCorrelation{}, err
	}
	defer rows.Close()

	var hours, quality, feel, focus []float64
	for rows.Next() {
		var h, avgFocus float64
		var q, f int
		if err := rows.Scan(&h, &q, &f, &avgFocus); err != nil {
			return tuiCorrelation{}, err
		}
		hours = append(hours, h)
		quality = append(quality, float64(q))
		feel = append(feel, float64(f))
		focus = append(focus, avgFocus)
	}
	if err := rows.Err(); err != nil {
		return tuiCorrelation{}, err
	}

	c := tuiCorrelation{pairedDays: len(hours)}
	if len(hours) >= 3 {
		c.rHours = pearson(hours, focus)
		c.rQuality = pearson(quality, focus)
		c.rFeel = pearson(feel, focus)
	}
	return c, nil
}

func rValueStr(r float64) string {
	return fmt.Sprintf("r=%5.2f", r)
}

func rStyle(r float64) lipgloss.Style {
	abs := math.Abs(r)
	switch {
	case abs > 0.6:
		return focusHighStyle.Padding(0, 1)
	case abs >= 0.3:
		return focusMidStyle.Padding(0, 1)
	default:
		return dimStyle.Padding(0, 1)
	}
}
func rLine(r float64) string {
	abs := math.Abs(r)
	style := dimStyle
	switch {
	case abs > 0.6:
		style = focusHighStyle
	case abs >= 0.3:
		style = focusMidStyle
	}
	return style.Render(fmt.Sprintf("r=%5.2f  %-10s", r, scaledBar(abs, 1.0, 10)))
}

type tuiWeeklyTrend struct {
	sleepHours [7]float64
	sleepHas   [7]bool
	focusAvg   [7]float64
	focusHas   [7]bool
}

func loadWeeklyTrend(weekStart string) (tuiWeeklyTrend, error) {
	var t tuiWeeklyTrend
	dates := weekDates(weekStart)
	idx := make(map[string]int, 7)
	for i, d := range dates {
		idx[d] = i
	}

	sleepRows, err := conn.Query(
		`SELECT date, sleep_hours, sleep_quality, feel, water_intake FROM daily_checkin WHERE date >= ? AND date <= ? ORDER BY date`,
		dates[0], dates[6],
	)
	if err != nil {
		return t, err
	}
	for sleepRows.Next() {
		var rawDate string
		var hours float64
		var quality, feel int
		var water sql.NullFloat64
		if err := sleepRows.Scan(&rawDate, &hours, &quality, &feel, &water); err != nil {
			sleepRows.Close()
			return t, err
		}
		d := normalizeDate(rawDate)
		if i, ok := idx[d]; ok {
			t.sleepHours[i] = hours
			t.sleepHas[i] = true
		}
	}
	if err := sleepRows.Err(); err != nil {
		sleepRows.Close()
		return t, err
	}
	sleepRows.Close()

	focusRows, err := conn.Query(
		`SELECT date, AVG(focus_quality) FROM blocks
		 WHERE date >= ? AND date <= ? AND focus_quality IS NOT NULL
		 GROUP BY date`,
		dates[0], dates[6],
	)
	if err != nil {
		return t, err
	}
	defer focusRows.Close()
	for focusRows.Next() {
		var date string
		var avg float64
		if err := focusRows.Scan(&date, &avg); err != nil {
			return t, err
		}
		if i, ok := idx[normalizeDate(date)]; ok {
			t.focusAvg[i] = avg
			t.focusHas[i] = true
		}
	}
	return t, focusRows.Err()
}
func normalizeDate(raw string) string {
	if pt, err := time.Parse(time.RFC3339, raw); err == nil {
		return pt.Format("2006-01-02")
	}
	if len(raw) >= 10 {
		return raw[:10]
	}
	return raw
}

var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// sparkline renders one glyph per day; days with no data show '·'.
func toPlotSeries(values [7]float64, has [7]bool) []float64 {
	out := make([]float64, 7)
	for i, v := range values {
		if has[i] {
			out[i] = v
		} else {
			out[i] = math.NaN()
		}
	}
	return out
}

func weekDayAxis(graph string) string {
	firstLine := graph
	if i := strings.IndexByte(graph, '\n'); i >= 0 {
		firstLine = graph[:i]
	}
	margin := strings.IndexRune(firstLine, '┤')
	if margin < 0 {
		margin = 0
	} else {
		margin++ // include the axis glyph itself
	}
	days := []string{"M", "T", "W", "T", "F", "S", "S"}
	return strings.Repeat(" ", margin) + strings.Join(days, "   ")
}

// weekGoalStats returns done/total goal counts for a week.
func weekGoalStats(w goalsWeek) (done, total int) {
	for _, d := range w.days {
		for _, g := range d.goals {
			total++
			if g.done {
				done++
			}
		}
	}
	return
}

const completionBarWidth = 5

func completionBar(done, total int) string {
	filled := int(math.Round(float64(done) / float64(total) * float64(completionBarWidth)))
	if filled > completionBarWidth {
		filled = completionBarWidth
	}
	bar := strings.Repeat("■", filled) + strings.Repeat(".", completionBarWidth-filled)

	return fmt.Sprintf("%s %d/%d done", bar, done, total)
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "OPEN":
		return okStyle
	case "CLOSED":
		return dimStyle
	default:
		return tableCellStyle
	}
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
		`SELECT date, sleep_hours, sleep_quality, feel, water_intake FROM daily_checkin WHERE date >= ? ORDER BY date DESC`,
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
		var water sql.NullFloat64
		if err := rows.Scan(&rawDate, &s.hours, &s.quality, &s.feel, &water); err != nil {

			return nil, err
		}
		if water.Valid {
			s.water = water.Float64
			s.hasWater = true
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
		SELECT b.id, b.date, b.block_num, b.day, p.name, b.outcome, b.context_reload,
		       b.deliverable, b.done_notes, b.not_done_notes, b.next_step, b.files_links,
		       b.focus_quality, b.tweak, b.created_at, b.closed_at
		FROM blocks b LEFT JOIN projects p ON p.id = b.project_id
		WHERE b.id = ?`, id,
	).Scan(&d.id, &d.date, &d.blockNum, &d.day, &project, &d.outcome, &d.contextReload,
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
			d, err := loadBlockDetail(b.id)
			if err != nil {
				m.err = err
				return m, nil
			}
			// if b.closed {
			// 	m.status = "That block is already closed."
			// 	return m, nil
			// }
			m.mode = "form"
			m.formPurpose = "block_update"
			m.f = newForm(fmt.Sprintf("Updating block #%d", b.blockNum), blockUpdateFieldLabels, []string{
				valOrEmpty(d.outcome), valOrEmpty(d.contextReload),
				valOrEmpty(d.deliverable), valOrEmpty(d.doneNotes), valOrEmpty(d.notDoneNotes),
				valOrEmpty(d.filesLinks),
			})
			m.f.ctxID = b.id
			m.f.field = 2
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
			waterStr := ""
			if s.hasWater {
				waterStr = strconv.FormatFloat(s.water, 'f', -1, 64)
			}
			m.f = newForm("Update sleep checkin", sleepFieldLabels, []string{
				strconv.FormatFloat(s.hours, 'f', -1, 64),
				strconv.Itoa(s.quality),
				strconv.Itoa(s.feel),
				waterStr,
				s.date,
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

	case "a":
		if m.tab == tabBlocks && len(m.blocks) > 0 {
			b := m.blocks[m.blockCur]
			m.mode = "form"
			m.formPurpose = "block_append_note"
			m.f = newForm(fmt.Sprintf("Append note — block #%d", b.blockNum), appendNoteFieldLabels, nil)
			m.f.ctxID = b.id
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
			// if b.closed {
			// 	m.status = "That block is already closed."
			// 	return m, nil
			// }
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
					if m.goalWeekExpanded == m.goalCurWeek {
						m.goalWeekExpanded = -1
					} else {
						m.goalWeekExpanded = m.goalCurWeek
					}
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
func valOrEmpty(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

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
	case "block_append_note":
		return m.submitBlockAppendNote()
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
	if outcome == "" {
		m.f.errMsg = "Fields are required."
		return m, nil
	}
	if m.openBlock != nil {
		m.f.errMsg = "A block is already open — close it first."
		return m, nil
	}

	today := time.Now().In(displayLoc).Format("2006-01-02")
	var nextNum int
	if err := conn.QueryRow(`SELECT COALESCE(MAX(block_num), 0) + 1 FROM blocks WHERE date = ?`, today).Scan(&nextNum); err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	_, err := conn.Exec(
		`INSERT INTO blocks (date, block_num, day, project_id, outcome, context_reload)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		today, nextNum, time.Now().In(displayLoc).Format("Mon"), m.f.ctxID, outcome, contextReload,
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
	outcome := strings.TrimSpace(m.f.values[0])
	contextReload := strings.TrimSpace(m.f.values[1])
	deliverable := strings.TrimSpace(m.f.values[2])
	doneNotes := strings.TrimSpace(m.f.values[3])
	notDoneNotes := strings.TrimSpace(m.f.values[4])
	filesLinks := strings.TrimSpace(m.f.values[5])

	if outcome == "" || contextReload == "" {
		m.f.errMsg = "Outcome, context reload can't be blank."
		return m, nil
	}

	id := m.f.ctxID
	_, err := conn.Exec(
		`UPDATE blocks SET
			outcome = ?, context_reload = ?, 
			deliverable = ?, done_notes = ?, not_done_notes = ?,
			files_links = ?
		 WHERE id = ?`,
		outcome, contextReload,
		deliverable, doneNotes, notDoneNotes,
		filesLinks, id,
	)

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
		weekStart = mondayOf(time.Now().In(displayLoc)).Format("2006-01-02")
	}

	day := "Mon"
	if weekStart == mondayOf(time.Now().In(displayLoc)).Format("2006-01-02") {
		day = time.Now().In(displayLoc).Format("Mon")
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
	hoursRaw := strings.TrimSpace(m.f.values[0])
	qualityRaw := strings.TrimSpace(m.f.values[1])
	feelRaw := strings.TrimSpace(m.f.values[2])
	waterRaw := strings.TrimSpace(m.f.values[3])
	dayRaw := strings.TrimSpace(m.f.values[4])

	day := time.Now().In(displayLoc).Format("2006-01-02")
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
	var water interface{}
	if waterRaw != "" {
		w, err := strconv.ParseFloat(waterRaw, 64)
		if err != nil || w < 0 || w > 10 {
			m.f.errMsg = "Water must be a number 0-10 (liters)."
			return m, nil
		}
		water = w
	}

	_, err = conn.Exec(
		`INSERT INTO daily_checkin (date, sleep_hours, sleep_quality, feel, water_intake, notes)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(date) DO UPDATE SET
			sleep_hours = excluded.sleep_hours,
			sleep_quality = excluded.sleep_quality,
feel = excluded.feel,
			water_intake = excluded.water_intake`,
		day, hours, quality, feel, water, "",
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
		return dimStyle.Render("○ No open block")
	}

	ob := m.openBlock
	elapsed := now.Sub(ob.startedAt)
	elapsed = max(elapsed, 0)
	endsAt := ob.startedAt.Add(time.Duration(blockLenMins) * time.Minute)
	remaining := endsAt.Sub(now)

	elapsedStr := formatDuration(elapsed)

	timerStyle := okStyle
	statusWord := fmt.Sprintf("ends %s", endsAt.Format("15:04"))
	if remaining < 0 {
		timerStyle = errStyle
		statusWord = fmt.Sprintf("over by %s", formatDuration(remaining))
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

func focusStyle(n int) lipgloss.Style {
	switch {
	case n <= 0:
		return tableCellStyle
	case n <= 3:
		return focusLowStyle
	case n <= 7:
		return focusMidStyle
	default:
		return focusHighStyle
	}
}

const clampColWidth = 15

var outcomeColWidth = int(float64(clampColWidth) * 1.2)

func dateOnly(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func (m tuiModel) viewBlocks() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("=== Blocks (this week) ==="))
	b.WriteString("\n")

	if len(m.blocks) == 0 {
		b.WriteString(dimStyle.Render("No blocks logged this week."))
		return b.String()
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tableBorderStyle).
		Headers("#", "Date", "Status", "Project", "Length", "Focus", "Outcome").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return tableHeaderStyle
			}
			blk := m.blocks[row]
			selected := row == m.blockCur
			switch col {
			case 2: // Status
				if selected {
					return rowSelectedStyle
				}
				status := "CLOSED"
				if !blk.closed {
					status = "OPEN"
				}
				return statusStyle(status).Padding(0, 1)
			case 5: // Focus
				if selected {
					return rowSelectedStyle.Align(lipgloss.Right)
				}
				if blk.focus != nil {
					return focusStyle(*blk.focus).Align(lipgloss.Right)
				}
				return tableCellStyle.Align(lipgloss.Right)
			default:
				if selected {
					return rowSelectedStyle
				}
				return tableCellStyle
			}
		})

	for i, blk := range m.blocks {
		marker := " "
		if i == m.blockCur {
			marker = "▶"
		}
		idStr := fmt.Sprintf("%s%*d", marker, 3, blk.blockNum)
		status := "CLOSED"
		if !blk.closed {
			status = "OPEN"
		}
		focus := "-"
		if blk.focus != nil {
			focus = strconv.Itoa(*blk.focus)
		}
		t.Row(idStr, dateOnly(blk.date), truncate(status, clampColWidth), truncate(blk.project, clampColWidth), truncate(blk.length, clampColWidth), truncate(focus, clampColWidth), truncate(blk.outcome, outcomeColWidth))
	}

	b.WriteString(t.Render())
	b.WriteString("\n")
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
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
		done, total := weekGoalStats(w)
		b.WriteString(fmt.Sprintf("%s%s Week %s – %s  #%d   %s\n",
			marker, arrow, w.weekStart, w.weekEnd, w.num, completionBar(done, total)))

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
			b.WriteString(dayMarker)
			b.WriteString(label)
			b.WriteString("\n")

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
	b.WriteString(headerStyle.Render("=== Wellness (this week) ==="))
	b.WriteString("\n")

	if len(m.sleep) == 0 {
		b.WriteString(dimStyle.Render("No checkins logged this week — press n to add one."))
		return b.String()
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tableBorderStyle).
		Headers("", "Date", "Sleep", "Quality", "Feel", "Water", "Day").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return tableHeaderStyle
			}
			s := m.sleep[row]
			selected := row == m.sleepCur
			switch col {
			case 3: // Quality
				if selected {
					return rowSelectedStyle
				}
				return focusStyle(s.quality)
			case 4: // Feel
				if selected {
					return rowSelectedStyle
				}
				return focusStyle(s.feel)
			default:
				if selected {
					return rowSelectedStyle
				}
				return tableCellStyle
			}
		})

	var sumHours, sumWater float64
	var sumQuality, sumFeel, waterN int
	for i, s := range m.sleep {
		marker := " "
		if i == m.sleepCur {
			marker = "▶"
		}
		waterStr := "-"
		if s.hasWater {
			waterStr = fmt.Sprintf("%.1fL", s.water)
			sumWater += s.water
			waterN++
		}
		t.Row(marker, s.date, fmt.Sprintf("%.1fh", s.hours), strconv.Itoa(s.quality), strconv.Itoa(s.feel), waterStr, s.weekday)

		sumHours += s.hours
		sumQuality += s.quality
		sumFeel += s.feel
	}
	b.WriteString(t.Render())
	b.WriteString("\n")
	n := float64(len(m.sleep))
	avgWaterStr := "-"
	if waterN > 0 {
		avgWaterStr = fmt.Sprintf("%.1fL", sumWater/float64(waterN))
	}
	b.WriteString(fmt.Sprintf("\nAvg sleep: %.1fh | Avg quality: %.1f | Avg feel: %.1f | Avg water: %s\n",
		sumHours/n, float64(sumQuality)/n, float64(sumFeel)/n, avgWaterStr))

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

	b.WriteString(headerStyle.Render("=== Weekly trend ==="))
	b.WriteString("\n")
	sleepGraph := asciigraph.Plot(
		toPlotSeries(m.weeklyTrend.sleepHours, m.weeklyTrend.sleepHas),
		asciigraph.Height(4),
		asciigraph.Width(20),
		asciigraph.Caption("Sleep (hrs)"),
	)
	b.WriteString(sleepGraph)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(weekDayAxis(sleepGraph)))
	b.WriteString("\n\n")

	focusGraph := asciigraph.Plot(
		toPlotSeries(m.weeklyTrend.focusAvg, m.weeklyTrend.focusHas),
		asciigraph.Height(4),
		asciigraph.Width(20),
		asciigraph.Caption("Focus (avg)"),
	)
	b.WriteString(focusGraph)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(weekDayAxis(focusGraph)))
	b.WriteString("\n")

	b.WriteString("\n")

	b.WriteString(headerStyle.Render("=== Blocks by project (this week) ==="))
	b.WriteString("\n")

	if len(m.metrics) == 0 {
		b.WriteString(dimStyle.Render("No blocks logged this week."))
	} else {
		for _, mt := range m.metrics {
			b.WriteString(fmt.Sprintf("  %-15s blocks:%-3d avg focus:%.1f\n", mt.project, mt.count, mt.avgFocus))
		}
	}

	b.WriteString("\n")
	b.WriteString(headerStyle.Render("=== Correlation (all-time, paired days) ==="))
	b.WriteString("\n")
	if m.correlation.pairedDays < 3 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Only %d paired days found. Need at least 3 (ideally 14+).\n", m.correlation.pairedDays)))
	} else {
		c := m.correlation
		rows := []struct {
			label string
			r     float64
		}{
			{"sleep_hours", c.rHours},
			{"sleep_quality", c.rQuality},
			{"feel", c.rFeel},
		}
		ct := table.New().
			Border(lipgloss.HiddenBorder()).
			StyleFunc(func(row, col int) lipgloss.Style {
				if col == 2 {
					return rStyle(rows[row].r)
				}
				return tableCellStyle
			})
		for _, row := range rows {
			ct.Row(row.label, "<-> focus", rValueStr(row.r), scaledBar(math.Abs(row.r), 1.0, 10))
		}
		b.WriteString(ct.Render())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("=== Focus by block position (all-time) ==="))
	b.WriteString("\n")
	if len(m.blockPosStats) == 0 {
		b.WriteString(dimStyle.Render("No closed blocks yet.\n"))
	} else {
		for _, s := range m.blockPosStats {
			b.WriteString(fmt.Sprintf("#%-3d %-8s %.1f   (%d blocks)\n",
				s.num, scaledBar(s.avg, 10, 7), s.avg, s.count))
		}
		b.WriteString(fmt.Sprintf("\nBest weekday: %-4s avg %.1f\n", m.weekdayBest.day, m.weekdayBest.avg))
		b.WriteString(fmt.Sprintf("Worst weekday: %-4s avg %.1f\n", m.weekdayWorst.day, m.weekdayWorst.avg))
	}

	b.WriteString(headerStyle.Render("=== Focus by start time (all-time) ==="))
	b.WriteString("\n")
	anyStartTime := false
	for _, s := range m.startTimeStats {
		if s.count > 0 {
			anyStartTime = true
			break
		}
	}
	if !anyStartTime {
		b.WriteString(dimStyle.Render("No closed blocks yet.\n"))
	} else {
		for _, s := range m.startTimeStats {
			if s.count == 0 {
				b.WriteString(fmt.Sprintf("%s %-8s  -    (0 blocks)\n", s.label, ""))
				continue
			}
			avg := float64(s.sum) / float64(s.count)
			b.WriteString(fmt.Sprintf("%s %-8s %.1f   (%d blocks)\n", s.label, scaledBar(avg, 10, 7), avg, s.count))
		}
	}

	b.WriteString("\n")
	b.WriteString(headerStyle.Render("=== Focus distribution (all-time) ==="))
	b.WriteString("\n")
	if m.focusN == 0 {
		b.WriteString(dimStyle.Render("No closed blocks yet.\n"))
	} else {
		labels := [5]string{" 1-2 ", " 3-4 ", " 5-6 ", " 7-8 ", "9-10 "}
		maxCount := 0
		for _, c := range m.focusHist {
			if c > maxCount {
				maxCount = c
			}
		}
		for i, c := range m.focusHist {
			b.WriteString(fmt.Sprintf("%s %-12s %d\n", labels[i], scaledBar(float64(c), float64(maxCount), 12), c))
		}
		b.WriteString(fmt.Sprintf("\nmedian: %.0f   mean: %.1f   (%d blocks)\n", m.focusMedian, m.focusMean, m.focusN))
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
		val := strings.ReplaceAll(m.f.values[i], "\n", dimStyle.Render("")+"\n"+strings.Repeat(" ", labelWidth+3))
		b.WriteString(fmt.Sprintf("%s%-*s %s\n", marker, labelWidth+1, label+":", val))
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

type tuiBlockPosStat struct {
	num   int
	avg   float64
	count int
}

func loadBlockPosStats() ([]tuiBlockPosStat, error) {
	rows, err := conn.Query(`
		SELECT block_num, AVG(focus_quality), COUNT(*)
		FROM blocks WHERE focus_quality IS NOT NULL
		GROUP BY block_num ORDER BY block_num`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tuiBlockPosStat
	for rows.Next() {
		var s tuiBlockPosStat
		if err := rows.Scan(&s.num, &s.avg, &s.count); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type tuiWeekdayStat struct {
	day string
	avg float64
}

func loadWeekdayExtremes() (best, worst tuiWeekdayStat, err error) {
	rows, err := conn.Query(`
		SELECT day, AVG(focus_quality)
		FROM blocks WHERE focus_quality IS NOT NULL
		GROUP BY day`)
	if err != nil {
		return best, worst, err
	}
	defer rows.Close()

	first := true
	for rows.Next() {
		var s tuiWeekdayStat
		if err := rows.Scan(&s.day, &s.avg); err != nil {
			return best, worst, err
		}
		if first || s.avg > best.avg {
			best = s
		}
		if first || s.avg < worst.avg {
			worst = s
		}
		first = false
	}
	return best, worst, rows.Err()
}

// loadFocusDistribution buckets all logged focus_quality values into
// [1-2,3-4,5-6,7-8,9-10] and returns median/mean alongside.
func loadFocusDistribution() (hist [5]int, median, mean float64, n int, err error) {
	rows, err := conn.Query(`SELECT focus_quality FROM blocks WHERE focus_quality IS NOT NULL ORDER BY focus_quality`)
	if err != nil {
		return hist, 0, 0, 0, err
	}
	defer rows.Close()

	var vals []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return hist, 0, 0, 0, err
		}
		vals = append(vals, v)
	}
	if err := rows.Err(); err != nil {
		return hist, 0, 0, 0, err
	}

	n = len(vals)
	if n == 0 {
		return hist, 0, 0, 0, nil
	}

	sum := 0
	for _, v := range vals {
		hist[(v-1)/2]++ // 1-2->0, 3-4->1, 5-6->2, 7-8->3, 9-10->4
		sum += v
	}
	mean = float64(sum) / float64(n)

	sort.Ints(vals)
	if n%2 == 1 {
		median = float64(vals[n/2])
	} else {
		median = float64(vals[n/2-1]+vals[n/2]) / 2
	}
	return hist, median, mean, n, nil
}

func (m tuiModel) submitBlockAppendNote() (tea.Model, tea.Cmd) {
	note := strings.TrimSpace(m.f.values[0])
	if note == "" {
		m.f.errMsg = "Note can't be blank."
		return m, nil
	}
	_, err := conn.Exec(
		`UPDATE blocks SET done_notes = CASE
			WHEN done_notes IS NULL OR done_notes = '' THEN ?
			ELSE done_notes || ' | ' || ?
		 END WHERE id = ?`,
		note, note, m.f.ctxID,
	)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}
	m.mode = "browse"
	m.status = "Note appended."
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

// scaledBar renders a bar of up to maxWidth chars for value out of max.
func scaledBar(value, max float64, maxWidth int) string {
	if max <= 0 {
		return ""
	}
	w := int(math.Round(value / max * float64(maxWidth)))
	w = min(w, maxWidth)
	if w < 0 {
		w = 0
	}
	return strings.Repeat("■", w)
}

type tuiStartTimeStat struct {
	label string
	sum   int
	count int
}

var startTimeBuckets = []struct {
	label      string
	start, end int
}{
	{"4-8  ", 4, 8},
	{"8-12 ", 8, 12},
	{"12-16", 12, 16},
	{"16-20", 16, 20},
	{"20-24", 20, 24},
}

func loadStartTimeStats() ([]tuiStartTimeStat, error) {
	rows, err := conn.Query(`SELECT created_at, focus_quality FROM blocks WHERE focus_quality IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sums := make([]int, len(startTimeBuckets))
	counts := make([]int, len(startTimeBuckets))

	for rows.Next() {
		var createdAt string
		var focus int
		if err := rows.Scan(&createdAt, &focus); err != nil {
			return nil, err
		}
		t, perr := time.Parse("2006-01-02 15:04:05", createdAt)
		if perr != nil {
			if t2, perr2 := time.Parse(time.RFC3339, createdAt); perr2 == nil {
				t = t2
			} else {
				continue
			}
		}
		hour := t.Hour()
		for i, bucket := range startTimeBuckets {
			if hour >= bucket.start && hour < bucket.end {
				sums[i] += focus
				counts[i]++
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]tuiStartTimeStat, len(startTimeBuckets))
	for i, bucket := range startTimeBuckets {
		out[i] = tuiStartTimeStat{label: bucket.label, sum: sums[i], count: counts[i]}
	}
	return out, nil
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
	status := statusStyle("OPEN").Render("OPEN")
	if d.closedAt != nil {
		closedDisplay := *d.closedAt
		if ct, err := parseTimestamp(*d.closedAt); err == nil {
			closedDisplay = ct.Format("2006-01-02 15:04")
		}
		status = statusStyle("CLOSED").Render("CLOSED") + " at " + closedDisplay
	}
	row("Status", status)
	durationStr := "-"
	if d.closedAt != nil {
		if ct, err1 := parseTimestamp(d.createdAt); err1 == nil {
			if ct2, err2 := parseTimestamp(*d.closedAt); err2 == nil {
				durationStr = formatDuration(ct2.Sub(ct))
			}
		}
	}
	row("Duration", durationStr)
	createdDisplay := d.createdAt
	if ct, err := parseTimestamp(d.createdAt); err == nil {
		createdDisplay = ct.Format("2006-01-02 15:04")
	}
	row("Created", createdDisplay)

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
