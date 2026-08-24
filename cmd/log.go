package cmd

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Log a shallow-work session that already happened, ending now",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		projectID, err := selectProject(reader)
		if err != nil {
			return err
		}

		outcome := askRequired(reader, "Outcome")
		description := askRequired(reader, "Description")
		hours := askFloat(reader, "Hours", 0.1, 24)

		end := time.Now().UTC()
		start := end.Add(-time.Duration(hours * float64(time.Hour)))
		today := end.Format("2006-01-02")

		var nextNum int
		err = conn.QueryRow(
			`SELECT COALESCE(MAX(block_num), 0) + 1 FROM blocks WHERE date = ?`,
			today,
		).Scan(&nextNum)
		if err != nil {
			return err
		}

		res, err := conn.Exec(
			`INSERT INTO blocks (date, block_num, day, project_id, outcome, done_notes, block_type, created_at, closed_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'shallow', ?, ?)`,
			today, nextNum, end.Format("Mon"), projectID, outcome, description,
			start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"),
		)
		if err != nil {
			return err
		}

		id, _ := res.LastInsertId()
		fmt.Printf("Shallow block #%d logged (id=%d): %.1fh %s -> %s\n",
			nextNum, id, hours, start.Format("15:04"), end.Format("15:04"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
}
