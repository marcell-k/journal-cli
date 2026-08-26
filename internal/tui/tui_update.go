package tui

import (
	"fmt"
	"github.com/charmbracelet/bubbletea"
	"strconv"
	"strings"
	"time"
)

var validDays = map[string]string{
	"mon": "Mon", "tue": "Tue", "wed": "Wed", "thu": "Thu",
	"fri": "Fri", "sat": "Sat", "sun": "Sun",
}

// ---------- tea.Model ----------

func (m tuiModel) Init() tea.Cmd { return tickEvery() }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case notesEditorMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.status = "Notes saved."
		}
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
	case "ctrl+j":
		if m.tab == tabGoals {
			m.moveGoal(1)
		}
		return m, nil
	case "ctrl+k":
		if m.tab == tabGoals {
			m.moveGoal(-1)
		}
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
			} else if len(m.blocks) > 0 {
				m.blockCur = len(m.blocks) - 1
			}
		case tabGoals:
			m.goalUp()
		case tabSleep:
			if m.sleepCur > 0 {
				m.sleepCur--
			} else if len(m.sleep) > 0 {
				m.sleepCur = len(m.sleep) - 1
			}
		case tabProjects, tabNotes:
			if m.projectCur > 0 {
				m.projectCur--
			} else if len(m.projects) > 0 {
				m.projectCur = len(m.projects) - 1
			}
		}
		return m, nil

	case "down", "j":
		switch m.tab {
		case tabBlocks:
			if m.blockCur < len(m.blocks)-1 {
				m.blockCur++
			} else if len(m.blocks) > 0 {
				m.blockCur = 0
			}
		case tabGoals:
			m.goalDown()
		case tabSleep:
			if m.sleepCur < len(m.sleep)-1 {
				m.sleepCur++
			} else if len(m.sleep) > 0 {
				m.blockCur = 0
			}
		case tabProjects, tabNotes:
			if m.projectCur < len(m.projects)-1 {
				m.projectCur++
			} else if len(m.projects) > 0 {
				m.projectCur = 0
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
			m.formPurpose = "block_start"
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

	// s: log shallow work — retroactive, already-closed block. Blocks tab only.
	case "s":
		if m.tab == tabBlocks {
			if len(m.projects) == 0 {
				m.status = "No projects yet — switch to the Projects tab and press n to add one."
				return m, nil
			}
			m.mode = "start_project"
			m.formPurpose = "block_log"
			m.projectCur = 0
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
			if d.blockType == "shallow" {
				m.mode = "form"
				m.formPurpose = "block_update_shallow"
				m.f = newForm(fmt.Sprintf("Updating shallow block #%d", b.blockNum), shallowUpdateFieldLabels, []string{
					valOrEmpty(d.outcome), valOrEmpty(d.doneNotes),
				})
				m.f.ctxID = b.id
			} else {
				m.mode = "form"
				m.formPurpose = "block_update"
				m.f = newForm(fmt.Sprintf("Updating block #%d", b.blockNum), blockUpdateFieldLabels, []string{
					valOrEmpty(d.outcome), valOrEmpty(d.contextReload),
					valOrEmpty(d.deliverable), valOrEmpty(d.doneNotes), valOrEmpty(d.notDoneNotes),
					valOrEmpty(d.filesLinks),
				})
				m.f.ctxID = b.id
				m.f.field = 2
			}
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
				strconv.FormatFloat(s.quality, 'f', -1, 64),
				strconv.FormatFloat(s.feel, 'f', -1, 64),
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
			if err := tuiDB.QueryRow(`SELECT COUNT(*) FROM blocks WHERE project_id = ?`, p.id).Scan(&count); err != nil {
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
			if _, err := tuiDB.Exec(`UPDATE weekly_goals SET done = ? WHERE id = ?`, newDone, g.id); err != nil {
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
		case tabNotes:
			if msg.String() == "enter" && len(m.projects) > 0 {
				p := m.projects[m.projectCur]
				return m, openNoteEditor(p.id)
			}
			return m, nil
		}
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
	case "block_update_shallow":
		return m.submitBlockUpdateShallow()
	case "block_log":
		return m.submitBlockLog()
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
		} else if len(m.projects) > 0 {
			m.projectCur = len(m.projects) - 1
		}
		return m, nil
	case "down", "j":
		if m.projectCur < len(m.projects)-1 {
			m.projectCur++
		} else if len(m.projects) > 0 {
			m.projectCur = 0
		}
		return m, nil
	case "enter":
		p := m.projects[m.projectCur]
		m.mode = "form"
		if m.formPurpose == "block_log" {
			m.f = newForm(fmt.Sprintf("Log shallow work — %s", p.name), blockLogFieldLabels, nil)
		} else {
			m.formPurpose = "block_start"
			m.f = newForm(fmt.Sprintf("New block — %s", p.name), blockStartFieldLabels, nil)
		}
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
	if err := tuiDB.QueryRow(`SELECT COALESCE(MAX(block_num), 0) + 1 FROM blocks WHERE date = ?`, today).Scan(&nextNum); err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	_, err := tuiDB.Exec(
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
		m.blockCur = 0
	}
	return m, nil
}

func (m tuiModel) submitBlockUpdateShallow() (tea.Model, tea.Cmd) {
	outcome := strings.TrimSpace(m.f.values[0])
	description := strings.TrimSpace(m.f.values[1])
	if outcome == "" {
		m.f.errMsg = "Outcome can't be blank."
		return m, nil
	}

	_, err := tuiDB.Exec(`UPDATE blocks SET outcome = ?, done_notes = ? WHERE id = ?`, outcome, description, m.f.ctxID)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = "Shallow block updated."
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
	}
	return m, nil
}

func (m tuiModel) submitBlockLog() (tea.Model, tea.Cmd) {
	outcome := strings.TrimSpace(m.f.values[0])
	description := strings.TrimSpace(m.f.values[1])
	hoursRaw := strings.TrimSpace(m.f.values[2])

	if outcome == "" {
		m.f.errMsg = "Outcome can't be blank."
		return m, nil
	}
	hours, err := strconv.ParseFloat(hoursRaw, 64)
	if err != nil || hours <= 0 || hours > 24 {
		m.f.errMsg = "Hours must be a number 0-24."
		return m, nil
	}

	end := time.Now().In(displayLoc)
	start := end.Add(-time.Duration(hours * float64(time.Hour)))
	today := end.Format("2006-01-02")

	var nextNum int
	if err := tuiDB.QueryRow(`SELECT COALESCE(MAX(block_num), 0) + 1 FROM blocks WHERE date = ?`, today).Scan(&nextNum); err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	_, err = tuiDB.Exec(
		`INSERT INTO blocks (date, block_num, day, project_id, outcome, done_notes, block_type, created_at, closed_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'shallow', ?, ?)`,
		today, nextNum, end.Format("Mon"), m.f.ctxID, outcome, description,
		start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		m.err = err
		m.mode = "browse"
		return m, nil
	}

	m.mode = "browse"
	m.status = fmt.Sprintf("Shallow block #%d logged (%.1fh).", nextNum, hours)
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
	} else {
		m.err = nil
		m.blockCur = 0
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
	_, err := tuiDB.Exec(
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

	focus, err := strconv.ParseFloat(focusRaw, 64)
	if err != nil || focus < 1 || focus > 10 {
		m.f.errMsg = "Focus quality must be a number 1-10."
		return m, nil
	}

	id := m.f.ctxID
	_, err = tuiDB.Exec(
		`UPDATE blocks
		 SET done_notes = ?, not_done_notes = ?, next_step = ?,
		     focus_quality = ?, tweak = ?,
		     closed_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		done, notDone, nextStep, focus, tweak, id,
	)
	if err == nil && filesLinks != "" {
		_, err = tuiDB.Exec(
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

	_, err := tuiDB.Exec(
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

	_, err := tuiDB.Exec(`UPDATE weekly_goals SET goal = ? WHERE id = ?`, text, m.f.ctxID)
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
	quality, err := strconv.ParseFloat(qualityRaw, 64)
	if err != nil || quality < 1 || quality > 10 {
		m.f.errMsg = "Quality must be a number 1-10."
		return m, nil
	}
	feel, err := strconv.ParseFloat(feelRaw, 64)
	if err != nil || feel < 1 || feel > 10 {
		m.f.errMsg = "Feel must be a number 1-10."
		return m, nil
	}
	var water any
	if waterRaw != "" {
		w, err := strconv.ParseFloat(waterRaw, 64)
		if err != nil || w < 0 || w > 10 {
			m.f.errMsg = "Water must be a number 0-10 (liters)."
			return m, nil
		}
		water = w
	}

	_, err = tuiDB.Exec(
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

	res, err := tuiDB.Exec(`INSERT OR IGNORE INTO projects (name) VALUES (?)`, name)
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

	_, err := tuiDB.Exec(`UPDATE projects SET name = ? WHERE id = ?`, newName, m.f.ctxID)
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
		_, err = tuiDB.Exec(`DELETE FROM blocks WHERE id = ?`, pd.id)
	case "goal":
		_, err = tuiDB.Exec(`DELETE FROM weekly_goals WHERE id = ?`, pd.id)
	case "sleep":
		_, err = tuiDB.Exec(`DELETE FROM daily_checkin WHERE date = ?`, pd.dateKey)
	case "project":
		_, err = tuiDB.Exec(`DELETE FROM projects WHERE id = ?`, pd.id)
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

func (m *tuiModel) moveGoal(delta int) {
	w := m.currentGoalWeek()
	if w == nil || m.goalCurDay < 0 || m.goalCurGoal < 0 {
		return
	}
	goals := w.days[m.goalCurDay].goals
	i, j := m.goalCurGoal, m.goalCurGoal+delta
	if j < 0 || j >= len(goals) {
		return
	}
	g1, g2 := goals[i], goals[j]

	if _, err := tuiDB.Exec(`UPDATE weekly_goals SET sort_order = ? WHERE id = ?`, g2.order, g1.id); err != nil {
		m.err = err
		return
	}
	if _, err := tuiDB.Exec(`UPDATE weekly_goals SET sort_order = ? WHERE id = ?`, g1.order, g2.id); err != nil {
		m.err = err
		return
	}

	m.err = nil
	if rerr := m.reload(); rerr != nil {
		m.err = rerr
		return
	}
	m.goalCurGoal = j
}
