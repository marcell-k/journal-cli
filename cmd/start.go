package cmd

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new block",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		projectID, err := selectProject(reader)
		if err != nil {
			return err
		}

		outcome := askRequired(reader, "Outcome")
		contextReload := askRequired(reader, "Context reload")
		firstAction := askRequired(reader, "First action")

		today := time.Now().Format("2006-01-02")

		var nextNum int
		err = conn.QueryRow(
			`SELECT COALESCE(MAX(block_num), 0) + 1 FROM blocks WHERE date = ?`,
			today,
		).Scan(&nextNum)
		if err != nil {
			return err
		}

		res, err := conn.Exec(
			`INSERT INTO blocks (date, block_num, day, project_id, outcome, context_reload, first_action)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			today,
			nextNum,
			time.Now().Format("Mon"),
			projectID,
			outcome,
			contextReload,
			firstAction,
		)
		if err != nil {
			return err
		}

		id, _ := res.LastInsertId()
		fmt.Printf("Block #%d started (id=%d)\n", nextNum, id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
