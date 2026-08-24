package tui

import (
	"database/sql"
	"fmt"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/guptarohit/asciigraph"
	"journal/internal/stats"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type tuiCorrelation struct {
	pairedDays              int
	rHours, rQuality, rFeel float64
}

func loadTUICorrelation() (tuiCorrelation, error) {
	rows, err := tuiDB.Query(`
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
		var q, f float64
		if err := rows.Scan(&h, &q, &f, &avgFocus); err != nil {
			return tuiCorrelation{}, err
		}
		hours = append(hours, h)
		quality = append(quality, q)
		feel = append(feel, f)
		focus = append(focus, avgFocus)
	}
	if err := rows.Err(); err != nil {
		return tuiCorrelation{}, err
	}

	c := tuiCorrelation{pairedDays: len(hours)}
	if len(hours) >= 3 {
		c.rHours = stats.Pearson(hours, focus)
		c.rQuality = stats.Pearson(quality, focus)
		c.rFeel = stats.Pearson(feel, focus)
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

	sleepRows, err := tuiDB.Query(
		`SELECT date, sleep_hours, sleep_quality, feel, water_intake FROM daily_checkin WHERE date >= ? AND date <= ? ORDER BY date`,
		dates[0], dates[6],
	)
	if err != nil {
		return t, err
	}
	for sleepRows.Next() {
		var rawDate string
		var hours float64
		var quality, feel float64
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

	focusRows, err := tuiDB.Query(
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
	if first, _, ok := strings.Cut(graph, "\n"); ok {
		firstLine = first
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
	filled = min(filled, completionBarWidth)
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
	for i := range count {
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
			fmt.Fprintf(&b, "  %-15s blocks:%-3d avg focus:%.1f\n", mt.project, mt.count, mt.avgFocus)
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
			fmt.Fprintf(&b, "#%-3d %-8s %.1f   (%d blocks)\n",
				s.num, scaledBar(s.avg, 10, 7), s.avg, s.count)
		}
		fmt.Fprintf(&b, "\nBest weekday: %-4s avg %.1f\n", m.weekdayBest.day, m.weekdayBest.avg)
		fmt.Fprintf(&b, "Worst weekday: %-4s avg %.1f\n", m.weekdayWorst.day, m.weekdayWorst.avg)
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
				fmt.Fprintf(&b, "%s %-8s  -    (0 blocks)\n", s.label, "")
				continue
			}
			avg := s.sum / float64(s.count)
			fmt.Fprintf(&b, "%s %-8s %.1f   (%d blocks)\n", s.label, scaledBar(avg, 10, 7), avg, s.count)
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
			fmt.Fprintf(&b, "%s %-12s %d\n", labels[i], scaledBar(float64(c), float64(maxCount), 12), c)
		}
		fmt.Fprintf(&b, "\nmedian: %.0f   mean: %.1f   (%d blocks)\n", m.focusMedian, m.focusMean, m.focusN)
	}
	return b.String()
}

// viewForm renders whatever form is currently active (new/update block,
// close block, add/edit goal, add/update sleep checkin, add/rename project).

type tuiBlockPosStat struct {
	num   int
	avg   float64
	count int
}

func loadBlockPosStats() ([]tuiBlockPosStat, error) {
	rows, err := tuiDB.Query(`
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
	rows, err := tuiDB.Query(`
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
	rows, err := tuiDB.Query(`SELECT focus_quality FROM blocks WHERE focus_quality IS NOT NULL ORDER BY focus_quality`)
	if err != nil {
		return hist, 0, 0, 0, err
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var v float64
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

	sum := 0.0
	for _, v := range vals {
		bucket := int(math.Floor((v - 1) / 2))
		bucket = min(max(bucket, 0), 4)
		hist[bucket]++ // 1-2->0, 3-4->1, 5-6->2, 7-8->3, 9-10->4
		sum += v
	}
	mean = sum / float64(n)

	sort.Float64s(vals)
	if n%2 == 1 {
		median = vals[n/2]
	} else {
		median = (vals[n/2-1] + vals[n/2]) / 2
	}
	return hist, median, mean, n, nil
}

func (m tuiModel) submitBlockAppendNote() (tea.Model, tea.Cmd) {
	note := strings.TrimSpace(m.f.values[0])
	if note == "" {
		m.f.errMsg = "Note can't be blank."
		return m, nil
	}
	_, err := tuiDB.Exec(
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
	sum   float64
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
	rows, err := tuiDB.Query(`SELECT created_at, focus_quality FROM blocks WHERE focus_quality IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sums := make([]float64, len(startTimeBuckets))
	counts := make([]int, len(startTimeBuckets))

	for rows.Next() {
		var createdAt string
		var focus float64
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
		fmt.Fprintf(&b, "%-16s %s\n", label+":", val)
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
		focus = strconv.FormatFloat(*d.focus, 'f', 1, 64)
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
