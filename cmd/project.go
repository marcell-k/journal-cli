package cmd

import (
	"bufio"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// selectProject prints the project list and asks the user to pick by number.
// Returns the chosen project's id.
func selectProject(reader *bufio.Reader) (int, error) {
	rows, err := conn.Query(`SELECT id, name FROM projects ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	ids := []int{}
	fmt.Println("Project:")
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return 0, err
		}
		ids = append(ids, id)
		fmt.Printf("  %d) %s\n", id, name)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for {
		fmt.Print("Choose number: ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		choice, err := strconv.Atoi(text)
		if err != nil {
			fmt.Println("Not a number, try again.")
			continue
		}
		if slices.Contains(ids, choice) {
			return choice, nil
		}
		fmt.Println("Not a valid project id, try again.")
	}
}

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage the list of projects/types",
}

var projectAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add a new project name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := conn.Exec(`INSERT OR IGNORE INTO projects (name) VALUES (?)`, args[0])
		if err != nil {
			return err
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			fmt.Printf("Project %q already exists.\n", args[0])
		} else {
			fmt.Printf("Project %q added.\n", args[0])
		}
		return nil
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		rows, err := conn.Query(`SELECT id, name FROM projects ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			fmt.Printf("%d) %s\n", id, name)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	projectCmd.AddCommand(projectAddCmd)
	projectCmd.AddCommand(projectListCmd)
	rootCmd.AddCommand(projectCmd)
}
