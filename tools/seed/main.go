// Seed script — populates journal.db with sample data for UI testing.
// Run: go run ./tools/seed
// Deletes nothing; if you want a clean slate, rm journal.db first.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	"journal/internal/db"
)

const targetBlocks = 50
const numWeeks = 4 // includes current partial week

var projectNames = []string{"work", "side-project", "learning"}

var outcomes = []string{
	"Ship block list/show commands", "Fix project delete guard", "Write seed script",
	"Refactor tui form handling", "Add correlate command", "Debug sqlite locking issue",
	"Draft README examples", "Clean up goal numbering logic", "Add focus quality color coding",
	"Wire up block detail view", "Investigate flaky test", "Sketch weekly review flow",
	"Review PR from teammate", "Plan next sprint", "Read paper on habit tracking",
	"Prep demo for standup", "Fix timezone bug in header bar", "Add sleep/feel correlation",
	"Polish tui styling", "Write unit tests for pearson()",
}
var contextReloads = []string{
	"Picked up from yesterday's plan", "Fresh start, no prior context needed",
	"Continuing from this morning's block", "Resuming after a meeting break",
	"Back from lunch, re-reading notes", "Cold start after a few days off this project",
}
var doneNotesPool = []string{
	"Core logic working, needs polish", "Done, tests passing", "Mostly done, one edge case left",
	"Got further than expected", "Solid progress, no blockers",
}
var notDoneNotesPool = []string{
	"Didn't get to the edge cases", "Ran out of time before tests", "Got derailed by a bug",
	"Deferred to next block", "-",
}
var nextSteps = []string{
	"Write the missing test case", "Wire up the new field in the UI", "Fix the edge case found today",
	"Start the next command", "Review and merge", "Re-read the spec, then implement",
}
var filesLinksPool = []string{"cmd/block.go", "internal/db/schema.sql", "cmd/tui.go", "", "", "README.md"}
var tweaksPool = []string{
	"", "", "Write the guard clause first", "Timebox research to 10 min", "Close distractions before starting",
	"Re-state the outcome out loud before typing",
}
var goalTextPool = []string{
	"Ship the block-review commands", "Get correlate command working end to end",
	"Clear the backlog of small bugs", "Write tests for the tui submit handlers",
	"Draft next week's plan", "Read one chapter of the deep work book",
	"Pair on the project-delete guard", "Clean up dead code in cmd/",
}

func mondayOf(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1))
}

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.Intn(len(xs))] }

func main() {
	conn, err := db.Open("./journal.db")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	projectIDs := seedProjects(conn)

	thisMonday := mondayOf(time.Now())
	startMonday := thisMonday.AddDate(0, 0, -7*(numWeeks-1))

	// build day list: startMonday .. today (skip future days)
	type day struct {
		date  time.Time
		count int
	}
	var days []day
	for d := startMonday; !d.After(time.Now()); d = d.AddDate(0, 0, 1) {
		days = append(days, day{date: d})
	}

	// weighted count per day, mean ~3, max 6
	weights := []int{1, 2, 3, 4, 3, 2, 1} // counts 0..6
	weightedPick := func() int {
		total := 0
		for _, w := range weights {
			total += w
		}
		roll := r.Intn(total)
		for i, w := range weights {
			if roll < w {
				return i
			}
			roll -= w
		}
		return 3
	}
	sum := 0
	for i := range days {
		days[i].count = weightedPick()
		sum += days[i].count
	}
	// nudge total toward targetBlocks
	for sum != targetBlocks && len(days) > 0 {
		i := r.Intn(len(days))
		if sum < targetBlocks && days[i].count < 6 {
			days[i].count++
			sum++
		} else if sum > targetBlocks && days[i].count > 0 {
			days[i].count--
			sum--
		} else if sum == targetBlocks {
			break
		}
	}

	weekGoalsSeeded := map[string]bool{}
	totalInserted := 0
	lastBlockID := int64(0)

	for _, d := range days {
		dateStr := d.date.Format("2006-01-02")
		dayAbbr := d.date.Format("Mon")
		weekStart := mondayOf(d.date).Format("2006-01-02")

		if !weekGoalsSeeded[weekStart] {
			seedWeekGoals(conn, r, weekStart)
			weekGoalsSeeded[weekStart] = true
		}

		// daily checkin — sleep/feel loosely correlated with that day's avg focus later,
		// so just generate plausible values now.
		sleepHours := 5.5 + r.Float64()*3.5 // 5.5-9.0
		sleepQuality := 3 + r.Intn(7)       // 3-9
		feel := 3 + r.Intn(7)               // 3-9
		_, err := conn.Exec(
			`INSERT INTO daily_checkin (date, sleep_hours, sleep_quality, feel, notes)
			 VALUES (?, ?, ?, ?, '')
			 ON CONFLICT(date) DO UPDATE SET sleep_hours=excluded.sleep_hours,
			   sleep_quality=excluded.sleep_quality, feel=excluded.feel`,
			dateStr, sleepHours, sleepQuality, feel,
		)
		if err != nil {
			log.Fatalf("checkin %s: %v", dateStr, err)
		}

		for n := 1; n <= d.count; n++ {
			projID := pick(r, projectIDs)
			focus := 1 + r.Intn(10)
			createdAt := d.date.Add(time.Duration(8+n) * time.Hour).Format("2006-01-02 15:04:05")
			closedAt := d.date.Add(time.Duration(8+n)*time.Hour + 90*time.Minute).Format("2006-01-02 15:04:05")

			isLast := d.date.Format("2006-01-02") == days[len(days)-1].date.Format("2006-01-02") && n == d.count
			var closedVal interface{} = closedAt
			var doneNotes, notDoneNotes, nextStep, tweak interface{} = pick(r, doneNotesPool), pick(r, notDoneNotesPool), pick(r, nextSteps), pick(r, tweaksPool)
			var focusVal interface{} = focus
			if isLast {
				// leave the very last block open, mid-session, to exercise the "open block" UI
				closedVal = nil
				doneNotes, notDoneNotes, nextStep, tweak = nil, nil, nil, nil
				focusVal = nil
			}

			res, err := conn.Exec(
				`INSERT INTO blocks
					(date, block_num, day, project_id, outcome, context_reload,
					 deliverable, done_notes, not_done_notes, next_step, files_links,
					 focus_quality, tweak, created_at, closed_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,?, ?, ?, ?)`,
				dateStr, n, dayAbbr, projID,
				pick(r, outcomes), pick(r, contextReloads),
				"", doneNotes, notDoneNotes, nextStep, pick(r, filesLinksPool),
				focusVal, tweak, createdAt, closedVal,
			)
			if err != nil {
				log.Fatalf("block %s #%d: %v", dateStr, n, err)
			}
			lastBlockID, _ = res.LastInsertId()
			totalInserted++
		}
	}

	fmt.Printf("Seeded %d blocks across %d days (%d weeks), last block id=%d.\n",
		totalInserted, len(days), numWeeks, lastBlockID)
}

func seedProjects(conn *sql.DB) []int {
	for _, name := range projectNames {
		if _, err := conn.Exec(`INSERT OR IGNORE INTO projects (name) VALUES (?)`, name); err != nil {
			log.Fatal(err)
		}
	}
	rows, err := conn.Query(`SELECT id FROM projects`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			log.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func seedWeekGoals(conn *sql.DB, r *rand.Rand, weekStart string) {
	days := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	n := 3 + r.Intn(3) // 3-5 goals per week
	for i := 0; i < n; i++ {
		day := pick(r, days)
		goal := pick(r, goalTextPool)
		done := r.Intn(3) == 0 // ~1/3 marked done
		_, err := conn.Exec(
			`INSERT INTO weekly_goals (week_start, day, goal, done) VALUES (?, ?, ?, ?)`,
			weekStart, day, goal, done,
		)
		if err != nil {
			log.Fatal(err)
		}
	}
}
