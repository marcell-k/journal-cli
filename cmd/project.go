package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

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

var projectRenameCmd = &cobra.Command{
	Use:   "rename <old name> <new name>",
	Short: "Rename a project",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName, newName := args[0], args[1]

		var id int
		err := conn.QueryRow(`SELECT id FROM projects WHERE name = ?`, oldName).Scan(&id)
		if err == sql.ErrNoRows {
			return fmt.Errorf("no project named %q", oldName)
		}
		if err != nil {
			return err
		}

		_, err = conn.Exec(`UPDATE projects SET name = ? WHERE id = ?`, newName, id)
		if err != nil {
			return fmt.Errorf("rename failed (name %q may already exist): %w", newName, err)
		}
		fmt.Printf("Renamed project %q to %q.\n", oldName, newName)
		return nil
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a project (only if no blocks reference it)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		var id int
		err := conn.QueryRow(`SELECT id FROM projects WHERE name = ?`, name).Scan(&id)
		if err == sql.ErrNoRows {
			return fmt.Errorf("no project named %q", name)
		}
		if err != nil {
			return err
		}

		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM blocks WHERE project_id = ?`, id).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("project %q has %d block(s) logged against it — rename it instead, or reassign those blocks first", name, count)
		}

		_, err = conn.Exec(`DELETE FROM projects WHERE id = ?`, id)
		if err != nil {
			return err
		}
		fmt.Printf("Deleted project %q.\n", name)
		return nil
	},
}

func init() {
	projectCmd.AddCommand(projectAddCmd, projectListCmd, projectRenameCmd, projectDeleteCmd)
	rootCmd.AddCommand(projectCmd)
}
