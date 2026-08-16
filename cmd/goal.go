package cmd

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var validDays = map[string]string{
	"mon": "Mon", "tue": "Tue", "wed": "Wed", "thu": "Thu",
	"fri": "Fri", "sat": "Sat", "sun": "Sun",
}

var goalDay string

var goalCmd = &cobra.Command{
	Use:   "goal",
	Short: "Manage weekly goals",
}

var goalAddCmd = &cobra.Command{
	Use:   "add <goal text>",
	Short: "Add a goal for this week (defaults to today)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		weekStart := mondayOf(time.Now()).Format("2006-01-02")
		goal := strings.Join(args, " ")

		day := time.Now().Format("Mon")
		if goalDay != "" {
			canonical, ok := validDays[strings.ToLower(goalDay)]
			if !ok {
				return fmt.Errorf("invalid day %q — use one of: mon tue wed thu fri sat sun", goalDay)
			}
			day = canonical
		}

		_, err := conn.Exec(
			`INSERT INTO weekly_goals (week_start, day, goal) VALUES (?, ?, ?)`,
			weekStart, day, goal,
		)
		if err != nil {
			return err
		}
		fmt.Printf("Goal added for %s.\n", day)
		return nil
	},
}

var goalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List this week's goals with reference numbers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printWeekGoals(conn)
	},
}

var goalDoneCmd = &cobra.Command{
	Use:   "done <n>",
	Short: "Mark goal #n done (number from 'journal goal list', not the DB id)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return fmt.Errorf("goal number must be a positive integer, got %q", args[0])
		}

		id, err := goalIDForNumber(n)
		if err != nil {
			return err
		}

		_, err = conn.Exec(`UPDATE weekly_goals SET done = 1 WHERE id = ?`, id)
		if err != nil {
			return err
		}
		fmt.Printf("Goal #%d marked done.\n", n)
		return nil
	},
}

var goalEditCmd = &cobra.Command{
	Use:   "edit <n> <new goal text>",
	Short: "Edit goal #n's text (number from 'journal goal list', not the DB id)",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return fmt.Errorf("goal number must be a positive integer, got %q", args[0])
		}
		newText := strings.Join(args[1:], " ")

		id, err := goalIDForNumber(n)
		if err != nil {
			return err
		}

		_, err = conn.Exec(`UPDATE weekly_goals SET goal = ? WHERE id = ?`, newText, id)
		if err != nil {
			return err
		}
		fmt.Printf("Goal #%d updated.\n", n)
		return nil
	},
}

var goalDeleteCmd = &cobra.Command{
	Use:   "delete <n>",
	Short: "Delete goal #n (number from 'journal goal list', not the DB id)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return fmt.Errorf("goal number must be a positive integer, got %q", args[0])
		}

		id, err := goalIDForNumber(n)
		if err != nil {
			return err
		}

		_, err = conn.Exec(`DELETE FROM weekly_goals WHERE id = ?`, id)
		if err != nil {
			return err
		}
		fmt.Printf("Goal #%d deleted.\n", n)
		return nil
	},
}

// goalIDForNumber resolves a 'journal goal list' display number (1-based, in
// list order) to the underlying DB id, for the current week.
func goalIDForNumber(n int) (int, error) {
	weekStart := mondayOf(time.Now()).Format("2006-01-02")
	var id int
	err := conn.QueryRow(
		`SELECT id FROM weekly_goals WHERE week_start = ? ORDER BY id LIMIT 1 OFFSET ?`,
		weekStart, n-1,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("no goal #%d this week — run 'journal goal list'", n)
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// printWeekGoals is shared by 'goal list' and 'week' so numbering always matches.
func printWeekGoals(conn *sql.DB) error {
	weekStart := mondayOf(time.Now()).Format("2006-01-02")
	rows, err := conn.Query(
		`SELECT day, goal, done FROM weekly_goals WHERE week_start = ? ORDER BY id`,
		weekStart,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		n++
		var day, goal string
		var done bool
		if err := rows.Scan(&day, &goal, &done); err != nil {
			return err
		}
		mark := " "
		if done {
			mark = "x"
		}
		fmt.Printf("%d) [%s] %-4s %s\n", n, mark, day, goal)
	}
	return rows.Err()
}

func init() {
	goalAddCmd.Flags().StringVarP(&goalDay, "day", "d", "", "day override (mon/tue/wed/thu/fri/sat/sun), defaults to today")
	goalCmd.AddCommand(goalAddCmd, goalDoneCmd, goalEditCmd, goalDeleteCmd, goalListCmd)
	rootCmd.AddCommand(goalCmd)
}
