package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"journal/internal/db"

	"github.com/spf13/cobra"
)

// conn is shared across subcommands, opened once in PersistentPreRunE.
var conn *sql.DB

var rootCmd = &cobra.Command{
	Use:   "journal",
	Short: "Personal 1.5hr block journal",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		conn, err = db.Open("./journal.db")
		return err
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if conn != nil {
			conn.Close()
		}
	},
}

// Execute runs the root command; call from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func askInt(reader *bufio.Reader, prompt string, min, max int) int {
	for {
		fmt.Printf("%s (%d-%d): ", prompt, min, max)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		val, err := strconv.Atoi(text)
		if err != nil {
			fmt.Println("Not a number, try again.")
			continue
		}
		if val < min || val > max {
			fmt.Printf("Must be between %d and %d, try again.\n", min, max)
			continue
		}
		return val
	}
}

func askRequired(reader *bufio.Reader, prompt string) string {
	for {
		fmt.Print(prompt + ": ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
		fmt.Println("This can't be blank, try again.")
	}
}

func askOptional(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt + " (leave blank to skip): ")
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}
