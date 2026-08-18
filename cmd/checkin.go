package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	sleepHours   float64
	sleepQuality int
	sleepFeel    int
	sleepDay     string
	sleepWater   float64
	sleepNotes   string
)

var sleepCmd = &cobra.Command{
	Use:   "sleep",
	Short: "Log daily sleep and feel",
}

var sleepLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Log sleep hours, sleep quality, and feel for a day (defaults to today)",
	RunE: func(cmd *cobra.Command, args []string) error {
		day := time.Now().Format("2006-01-02")
		if sleepDay != "" {
			if _, err := time.Parse("2006-01-02", sleepDay); err != nil {
				return fmt.Errorf("invalid --day %q, expected YYYY-MM-DD", sleepDay)
			}
			day = sleepDay
		}

		reader := bufio.NewReader(os.Stdin)

		hours := sleepHours
		if !cmd.Flags().Changed("hours") {
			hours = askFloat(reader, "Sleep hours", 0, 24)
		} else if hours < 0 || hours > 24 {
			return fmt.Errorf("--hours must be between 0 and 24, got %v", hours)
		}

		quality := sleepQuality
		if !cmd.Flags().Changed("quality") {
			quality = askInt(reader, "Sleep quality", 1, 10)
		} else if quality < 1 || quality > 10 {
			return fmt.Errorf("--quality must be between 1 and 10, got %d", quality)
		}

		feel := sleepFeel
		if !cmd.Flags().Changed("feel") {
			feel = askInt(reader, "Feel", 1, 10)
		} else if feel < 1 || feel > 10 {
			return fmt.Errorf("--feel must be between 1 and 10, got %d", feel)
		}
		water := sleepWater
		if !cmd.Flags().Changed("water") {
			water = askFloat(reader, "Water intake (L)", 0, 10)
		} else if water < 0 || water > 10 {
			return fmt.Errorf("--water must be between 0 and 10, got %v", water)
		}

		_, err := conn.Exec(
			`INSERT INTO daily_checkin (date, sleep_hours, sleep_quality, feel, water_intake, notes)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(date) DO UPDATE SET
				sleep_hours = excluded.sleep_hours,
				sleep_quality = excluded.sleep_quality,
				feel = excluded.feel,
				water_intake = excluded.water_intake,
				notes = CASE WHEN excluded.notes = '' THEN daily_checkin.notes ELSE excluded.notes END`,
			day, hours, quality, feel, water, sleepNotes,
		)
		if err != nil {
			return err
		}

		fmt.Printf("Checkin saved for %s: sleep=%.1fh quality=%d feel=%d water=%.1fL\n", day, hours, quality, feel, water)
		return nil
	},
}

// askFloat prompts until a value within [min, max] is entered.
func askFloat(reader *bufio.Reader, prompt string, min, max float64) float64 {
	for {
		fmt.Printf("%s (%.0f-%.0f): ", prompt, min, max)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		val, err := strconv.ParseFloat(text, 64)
		if err != nil {
			fmt.Println("Not a number, try again.")
			continue
		}
		if val < min || val > max {
			fmt.Printf("Must be between %.0f and %.0f, try again.\n", min, max)
			continue
		}
		return val
	}
}

func init() {
	sleepLogCmd.Flags().Float64Var(&sleepHours, "hours", 0, "hours of sleep")
	sleepLogCmd.Flags().IntVar(&sleepQuality, "quality", 0, "sleep quality 1-10")
	sleepLogCmd.Flags().IntVar(&sleepFeel, "feel", 0, "how you feel 1-10")
	sleepLogCmd.Flags().Float64Var(&sleepWater, "water", 0, "water intake in liters")
	sleepLogCmd.Flags().StringVar(&sleepDay, "day", "", "day to log for (YYYY-MM-DD), defaults to today")
	sleepLogCmd.Flags().StringVar(&sleepNotes, "notes", "", "optional notes")

	sleepCmd.AddCommand(sleepLogCmd)
	rootCmd.AddCommand(sleepCmd)
}
