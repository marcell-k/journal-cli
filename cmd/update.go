package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update done notes / files-links on the currently open block, mid-session",
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

		doneNotes := askOptional(reader, "Done notes")
		deliverable := askOptional(reader, "Deliverable/checkpoint")
		filesLinks := askOptional(reader, "Files/links")

		if doneNotes == "" && deliverable == "" && filesLinks == "" {
			fmt.Println("Nothing entered, no changes made.")
			return nil
		}
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if doneNotes != "" {
			_, err = tx.Exec(
				`UPDATE blocks SET done_notes = CASE
					WHEN done_notes IS NULL OR done_notes = '' THEN ?
					ELSE done_notes || ' | ' || ?
				 END WHERE id = ?`,
				doneNotes, doneNotes, id,
			)
			if err != nil {
				return err
			}
		}
		if deliverable != "" {
			_, err = tx.Exec(
				`UPDATE blocks SET deliverable = CASE
					WHEN deliverable IS NULL OR deliverable = '' THEN ?
					ELSE deliverable || ' | ' || ?
				 END WHERE id = ?`,
				deliverable, deliverable, id,
			)
			if err != nil {
				return err
			}
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

		fmt.Printf("Block #%d updated (id=%d)\n", blockNum, id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
