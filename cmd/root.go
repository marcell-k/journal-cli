package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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

func parseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
