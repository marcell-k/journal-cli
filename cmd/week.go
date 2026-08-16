package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var weekCmd = &cobra.Command{
	Use:   "week",
	Short: "Show current week's goals and blocks",
	RunE: func(cmd *cobra.Command, args []string) error {
		weekStart := mondayOf(time.Now()).Format("2006-01-02")

		fmt.Println("=== Weekly Goals ===")
		if err := printWeekGoals(conn); err != nil {
			return err
		}

		fmt.Println("\n=== Blocks ===")
		blockRows, err := conn.Query(
			`SELECT date, block_num, outcome, focus_quality, next_step
			 FROM blocks
			 WHERE date >= ?
			 ORDER BY date, block_num`,
			weekStart,
		)
		if err != nil {
			return err
		}
		defer blockRows.Close()

		for blockRows.Next() {
			var date, outcome, nextStep string
			var blockNum int
			var focus *int
			if err := blockRows.Scan(&date, &blockNum, &outcome, &focus, &nextStep); err != nil {
				return err
			}
			focusStr := "-"
			if focus != nil {
				focusStr = fmt.Sprintf("%d", *focus)
			}
			fmt.Printf("%s #%d  focus:%s  %s  -> next: %s\n", date, blockNum, focusStr, outcome, nextStep)
		}
		if err := blockRows.Err(); err != nil {
			return err
		}

		return nil
	},
}

// mondayOf returns the Monday of the week containing t.
func mondayOf(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 { // Sunday
		weekday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1))
}

func init() {
	rootCmd.AddCommand(weekCmd)
}
