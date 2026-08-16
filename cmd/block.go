package cmd

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var (
	blockFrom    string
	blockTo      string
	blockProject string
)

var blockCmd = &cobra.Command{
	Use:   "block",
	Short: "View individual blocks (past or present)",
}

var blockListCmd = &cobra.Command{
	Use:   "list",
	Short: "List blocks, optionally filtered by date range or project",
	RunE: func(cmd *cobra.Command, args []string) error {
		query := `
			SELECT b.id, b.date, b.block_num, p.name, b.outcome, b.focus_quality, b.closed_at
			FROM blocks b LEFT JOIN projects p ON p.id = b.project_id
			WHERE 1=1`
		var params []interface{}

		if blockFrom != "" {
			if _, err := time.Parse("2006-01-02", blockFrom); err != nil {
				return fmt.Errorf("invalid --from %q, expected YYYY-MM-DD", blockFrom)
			}
			query += " AND b.date >= ?"
			params = append(params, blockFrom)
		}
		if blockTo != "" {
			if _, err := time.Parse("2006-01-02", blockTo); err != nil {
				return fmt.Errorf("invalid --to %q, expected YYYY-MM-DD", blockTo)
			}
			query += " AND b.date <= ?"
			params = append(params, blockTo)
		}
		if blockProject != "" {
			query += " AND p.name = ?"
			params = append(params, blockProject)
		}
		query += " ORDER BY b.date, b.block_num"

		rows, err := conn.Query(query, params...)
		if err != nil {
			return err
		}
		defer rows.Close()

		n := 0
		for rows.Next() {
			var id, blockNum int
			var date, outcome string
			var project sql.NullString
			var focus sql.NullInt64
			var closedAt sql.NullString
			if err := rows.Scan(&id, &date, &blockNum, &project, &outcome, &focus, &closedAt); err != nil {
				return err
			}
			status := "open"
			if closedAt.Valid {
				status = "closed"
			}
			focusStr := "-"
			if focus.Valid {
				focusStr = strconv.FormatInt(focus.Int64, 10)
			}
			proj := "-"
			if project.Valid {
				proj = project.String
			}
			fmt.Printf("id=%-4d %s #%-2d [%-6s] proj:%-10s focus:%-2s %s\n", id, date, blockNum, status, proj, focusStr, outcome)
			n++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if n == 0 {
			fmt.Println("No blocks found for that filter.")
		}
		return nil
	},
}

var blockShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show full detail for a single block",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("block id must be an integer, got %q", args[0])
		}

		var date, day, outcome, contextReload, firstAction string
		var project sql.NullString
		var deliverable, doneNotes, notDoneNotes, nextStep, filesLinks, tweak sql.NullString
		var focus sql.NullInt64
		var createdAt string
		var closedAt sql.NullString
		var blockNum int

		err = conn.QueryRow(`
			SELECT b.date, b.block_num, b.day, p.name, b.outcome, b.context_reload, b.first_action,
			       b.deliverable, b.done_notes, b.not_done_notes, b.next_step, b.files_links,
			       b.focus_quality, b.tweak, b.created_at, b.closed_at
			FROM blocks b LEFT JOIN projects p ON p.id = b.project_id
			WHERE b.id = ?`, id,
		).Scan(&date, &blockNum, &day, &project, &outcome, &contextReload, &firstAction,
			&deliverable, &doneNotes, &notDoneNotes, &nextStep, &filesLinks,
			&focus, &tweak, &createdAt, &closedAt)
		if err == sql.ErrNoRows {
			return fmt.Errorf("no block with id %d", id)
		}
		if err != nil {
			return err
		}

		printField := func(label string, v sql.NullString) {
			val := "-"
			if v.Valid && v.String != "" {
				val = v.String
			}
			fmt.Printf("%-16s %s\n", label+":", val)
		}

		projName := "-"
		if project.Valid {
			projName = project.String
		}

		fmt.Printf("Block #%d (id=%d) — %s (%s)\n", blockNum, id, date, day)
		fmt.Printf("%-16s %s\n", "Project:", projName)
		fmt.Printf("%-16s %s\n", "Outcome:", outcome)
		fmt.Printf("%-16s %s\n", "Context reload:", contextReload)
		fmt.Printf("%-16s %s\n", "First action:", firstAction)
		printField("Deliverable", deliverable)
		printField("Done", doneNotes)
		printField("Not done", notDoneNotes)
		printField("Next step", nextStep)
		printField("Files/links", filesLinks)
		focusStr := "-"
		if focus.Valid {
			focusStr = strconv.FormatInt(focus.Int64, 10)
		}
		fmt.Printf("%-16s %s\n", "Focus quality:", focusStr)
		printField("Tweak", tweak)
		status := "open"
		if closedAt.Valid {
			status = "closed at " + closedAt.String
		}
		durationStr := "-"
		if closedAt.Valid {
			ct, err1 := parseTimestamp(createdAt)
			ct2, err2 := parseTimestamp(closedAt.String)
			if err1 == nil && err2 == nil {
				durationStr = formatDuration(ct2.Sub(ct))
			}
		}
		fmt.Printf("%-16s %s\n", "Duration:", durationStr)
		fmt.Printf("%-16s %s\n", "Status:", status)
		fmt.Printf("%-16s %s\n", "Created:", createdAt)
		return nil
	},
}

func init() {
	blockListCmd.Flags().StringVar(&blockFrom, "from", "", "only show blocks on/after this date (YYYY-MM-DD)")
	blockListCmd.Flags().StringVar(&blockTo, "to", "", "only show blocks on/before this date (YYYY-MM-DD)")
	blockListCmd.Flags().StringVar(&blockProject, "project", "", "only show blocks for this project name")

	blockCmd.AddCommand(blockListCmd, blockShowCmd)
	rootCmd.AddCommand(blockCmd)
}
