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

	nextSteps, err := lastNextSteps(conn)
	if err != nil {
		return 0, err
	}

	type projectRow struct {
		id   int
		name string
	}
	var projects []projectRow
	maxNameLen := 0
	for rows.Next() {
		var p projectRow
		if err := rows.Scan(&p.id, &p.name); err != nil {
			return 0, err
		}
		projects = append(projects, p)
		if len(p.name) > maxNameLen {
			maxNameLen = len(p.name)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	ids := []int{}
	fmt.Println("Project:")
	for _, p := range projects {
		ids = append(ids, p.id)
		if ns, ok := nextSteps[p.id]; ok && ns != "" {
			fmt.Printf("  %d) %-*s  %s\n", p.id, maxNameLen, p.name, dim("("+ns+")"))
		} else {
			fmt.Printf("  %d) %s\n", p.id, p.name)
		}
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

func lastNextSteps(conn *sql.DB) (map[int]string, error) {
	rows, err := conn.Query(`
		SELECT project_id, next_step FROM blocks
		WHERE id IN (
			SELECT MAX(id) FROM blocks
			WHERE next_step IS NOT NULL AND next_step != '' AND project_id IS NOT NULL
			GROUP BY project_id
		)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int]string{}
	for rows.Next() {
		var pid int
		var ns string
		if err := rows.Scan(&pid, &ns); err != nil {
			return nil, err
		}
		out[pid] = ns
	}
	return out, rows.Err()
}

func dim(s string) string {
	// return "\x1b[2m" + s + "\x1b[0m"
	return "\x1b[3;2m" + s + "\x1b[0m"
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
