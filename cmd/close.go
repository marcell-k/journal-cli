package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:   "close",
	Short: "Close the most recent open block",
	RunE: func(cmd *cobra.Command, args []string) error {
		var id int
		var blockNum int
		err := conn.QueryRow(
			`SELECT id, block_num FROM blocks WHERE closed_at IS NULL ORDER BY id DESC LIMIT 1`,
		).Scan(&id, &blockNum)
		if err == sql.ErrNoRows {
			fmt.Println("No open block found. Run 'journal start' first.")
			return nil
		}
		if err != nil {
			return err
		}

		reader := bufio.NewReader(os.Stdin)

		done := askRequired(reader, "Done")
		notDone := askRequired(reader, "Not done")
		nextStep := askRequired(reader, "Exact next step to start with")
		filesLinks := askOptional(reader, "Files/links")
		focus := askFloat(reader, "Focus quality", 1, 10)
		tweak := askOptional(reader, "One tweak for next block")

		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		_, err = tx.Exec(
			`UPDATE blocks
			 SET done_notes = ?, not_done_notes = ?, next_step = ?,
			     focus_quality = ?, tweak = ?,
			     closed_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			done, notDone, nextStep, focus, tweak, id,
		)
		if err != nil {
			return err
		}

		if filesLinks != "" {
			_, err = tx.Exec(
				`UPDATE blocks SET files_links = CASE
					WHEN files_links IS NULL OR files_links = '' THEN ?
					ELSE files_links || ' | ' || ?
				 END WHERE id = ?`,
				filesLinks, filesLinks, id,
			)
			if err != nil {
				return err
			}
		}

		if err := tx.Commit(); err != nil {
			return err
		}
		fmt.Printf("Block #%d closed (id=%d)\n", blockNum, id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(closeCmd)
}
