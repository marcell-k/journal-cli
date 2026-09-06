package tui

import (
	"database/sql"
	"time"
)

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
	rangeStart := mondayOf(time.Now().In(displayLoc)).AddDate(0, 0, -7*(weeksToShow-1)).Format("2006-01-02")

	blocks, err := loadTUIBlocks(rangeStart)
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

	sleep, err := loadTUISleep(rangeStart)
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

	weeklyTrend, err := loadWeeklyTrend(rangeStart)
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
	err := tuiDB.QueryRow(`
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
	rows, err := tuiDB.Query(`
		SELECT b.id, b.date, b.block_num, b.day, p.name, b.outcome, b.block_type, b.focus_quality, b.created_at, b.closed_at
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
		var focus *float64
		var createdAt string
		var closedAt *string
		if err := rows.Scan(&b.id, &b.date, &b.blockNum, &b.day, &project, &b.outcome, &b.blockType, &focus, &createdAt, &closedAt); err != nil {
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
				b.length = formatDuration(time.Since(ct))
			}
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func weekDates(weekStart string) [7]string {
	var out [7]string
	t, _ := time.Parse("2006-01-02", weekStart)
	for i := range 7 {
		out[i] = t.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

func rangeDates(start string, numDays int) []string {
	out := make([]string, numDays)
	t, _ := time.Parse("2006-01-02", start)
	for i := range numDays {
		out[i] = t.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

func firstWeekStart() (string, error) {
	var s sql.NullString
	if err := tuiDB.QueryRow(`SELECT MIN(week_start) FROM weekly_goals`).Scan(&s); err != nil {
		return "", err
	}
	if !s.Valid {
		return "", nil
	}
	return s.String, nil
}

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

	rows, err := tuiDB.Query(
		`SELECT id, day, goal, done, COALESCE(sort_order, id) FROM weekly_goals WHERE week_start = ? ORDER BY COALESCE(sort_order, id)`,
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
		if err := rows.Scan(&g.id, &g.day, &g.goal, &g.done, &g.order); err != nil {
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

func loadTUISleep(weekStart string) ([]tuiSleep, error) {
	rows, err := tuiDB.Query(
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
	rows, err := tuiDB.Query(`
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
	rows, err := tuiDB.Query(`
		SELECT p.id, p.name, ns.next_step
		FROM projects p
		LEFT JOIN (
			SELECT project_id, next_step FROM blocks
			WHERE id IN (
				SELECT MAX(id) FROM blocks
				WHERE next_step IS NOT NULL AND next_step != '' AND project_id IS NOT NULL
				GROUP BY project_id
			)
		) ns ON ns.project_id = p.id
		ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tuiProject
	for rows.Next() {
		var p tuiProject
		var nextStep sql.NullString
		if err := rows.Scan(&p.id, &p.name, &nextStep); err != nil {
			return nil, err
		}
		if nextStep.Valid {
			p.nextStep = nextStep.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadBlockDetail(id int) (tuiBlockDetail, error) {
	var d tuiBlockDetail
	var project sql.NullString
	var outcome, contextReload sql.NullString
	var deliverable, doneNotes, notDoneNotes, nextStep, filesLinks, tweak sql.NullString
	var focus sql.NullFloat64
	var closedAt sql.NullString

	err := tuiDB.QueryRow(`
		SELECT b.id, b.date, b.block_num, b.day, p.name, b.outcome, b.context_reload,
		       b.deliverable, b.done_notes, b.not_done_notes, b.next_step, b.files_links,
		       b.focus_quality, b.tweak, b.created_at, b.closed_at, b.block_type
		FROM blocks b LEFT JOIN projects p ON p.id = b.project_id
		WHERE b.id = ?`, id,
	).Scan(&d.id, &d.date, &d.blockNum, &d.day, &project, &outcome, &contextReload,
		&deliverable, &doneNotes, &notDoneNotes, &nextStep, &filesLinks,
		&focus, &tweak, &d.createdAt, &closedAt, &d.blockType)
	if err != nil {
		return d, err
	}

	d.project = "-"
	if project.Valid {
		d.project = project.String
	}
	d.outcome = nullOr(outcome)
	d.contextReload = nullOr(contextReload)
	d.deliverable = nullOr(deliverable)
	d.doneNotes = nullOr(doneNotes)
	d.notDoneNotes = nullOr(notDoneNotes)
	d.nextStep = nullOr(nextStep)
	d.filesLinks = nullOr(filesLinks)
	d.tweak = nullOr(tweak)
	if focus.Valid {
		f := focus.Float64
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
