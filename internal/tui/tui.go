package tui

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------- tabs ----------

const (
	tabBlocks = iota
	tabGoals
	tabSleep
	tabProjects
	tabNotes
	tabMetrics
	tabCount
)

const weeksToShow = 3

const notesDir = "notes"

var tabNames = [tabCount]string{"Blocks", "Goals", "Wellness", "Projects", "Notes", "Metrics"}

type notesEditorMsg struct{ err error }

func openNoteEditor(projectID int) tea.Cmd {
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return func() tea.Msg { return notesEditorMsg{err: err} }
	}
	path := filepath.Join(notesDir, fmt.Sprintf("%d.md", projectID))
	c := exec.Command("nvim", path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return notesEditorMsg{err: err}
	})
}

func hasNotes(projectID int) bool {
	path := filepath.Join(notesDir, fmt.Sprintf("%d.md", projectID))
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

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

	deepTypeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7fb4ca"))
	shallowTypeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6c384"))

	bannerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7fb4ca")).
			Border(lipgloss.RoundedBorder()).Padding(0, 1).MarginTop(1).MarginBottom(1)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6c384"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6a69c"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#e46876"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#87a987"))
)

// ---------- block-type ----------
func typeGlyph(blockType string) string {
	if blockType == "shallow" {
		return shallowTypeStyle.Render("●")
	}
	return deepTypeStyle.Render("●")
}

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
	blockType    string
	focus        *float64
	length       string
	closed       bool
}

type tuiGoal struct {
	id, num int
	day     string
	goal    string
	done    bool
	order   int
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
	quality, feel float64
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
	blockLogFieldLabels      = []string{"Outcome", "Description", "Hours"}
	shallowUpdateFieldLabels = []string{"Outcome", "Description"}
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
	blockType                            string

	focus *float64

	createdAt string
	closedAt  *string
}

// ---------- model ----------

var tuiDB *sql.DB

// Run starts the TUI using the provided database connection.
func Run(db *sql.DB) error {
	tuiDB = db
	m, err := newTUIModel()
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func parseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func mondayOf(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1))
}
