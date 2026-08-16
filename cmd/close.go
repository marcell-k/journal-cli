package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strings"

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
		ask := func(prompt string) string {
			fmt.Print(prompt + ": ")
			text, _ := reader.ReadString('\n')
			return strings.TrimSpace(text)
		}

		done := ask("Done")
		notDone := ask("Not done")
		nextStep := ask("Exact next step to start with")
		filesLinks := ask("Files/links (leave blank to skip)")
		focus := askInt(reader, "Focus quality", 1, 5)
		tweak := ask("One tweak for next block")

		_, err = conn.Exec(
			`UPDATE blocks
			 SET done_notes = ?, not_done_notes = ?, next_step = ?,
			     focus_quality = ?, tweak = ?,
			     closed_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			done, notDone, nextStep, focus, tweak, id,
		)

		if filesLinks != "" {
			_, err = conn.Exec(
				`UPDATE blocks SET files_links = CASE
					WHEN files_links IS NULL OR files_links = '' THEN ?
					ELSE files_links || ' | ' || ?
				 END WHERE id = ?`,
				filesLinks, filesLinks, id,
			)
		}
		if err != nil {
			return err
		}

		fmt.Printf("Block #%d closed (id=%d)\n", blockNum, id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(closeCmd)
}
