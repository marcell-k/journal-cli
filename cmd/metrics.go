package cmd

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/spf13/cobra"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Show productivity and wellbeing stats",
}

var metricsWeekCmd = &cobra.Command{
	Use:   "week",
	Short: "Show this week's block/focus stats by project",
	RunE: func(cmd *cobra.Command, args []string) error {
		weekStart := mondayOf(time.Now()).Format("2006-01-02")
		rows, err := conn.Query(`
			SELECT p.name, COUNT(*), AVG(b.focus_quality)
			FROM blocks b JOIN projects p ON p.id = b.project_id
			WHERE b.date >= ?
			GROUP BY p.name ORDER BY COUNT(*) DESC`, weekStart)
		if err != nil {
			return err
		}
		defer rows.Close()

		fmt.Println("=== Blocks by project (this week) ===")
		for rows.Next() {
			var name string
			var count int
			var avgFocus sql.NullFloat64
			if err := rows.Scan(&name, &count, &avgFocus); err != nil {
				return err
			}
			fmt.Printf("%-15s blocks:%-3d avg focus:%.1f\n", name, count, avgFocus.Float64)
		}
		return rows.Err()
	},
}

var metricsSleepCmd = &cobra.Command{
	Use:   "sleep",
	Short: "Show sleep/feel daily log and weekly averages",
	RunE: func(cmd *cobra.Command, args []string) error {
		weekStart := mondayOf(time.Now()).Format("2006-01-02")

		rows, err := conn.Query(
			`SELECT date, sleep_hours, sleep_quality, feel, water_intake
			 FROM daily_checkin WHERE date >= ? ORDER BY date`,
			weekStart,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		fmt.Println("=== Daily checkins (this week) ===")
		var sumHours, sumWater float64
		var sumQuality, sumFeel float64
		var waterN, n int
		for rows.Next() {
			var rawDate string
			var hours, quality, feel float64
			var water sql.NullFloat64
			if err := rows.Scan(&rawDate, &hours, &quality, &feel, &water); err != nil {
				return err
			}
			displayDate := rawDate
			if t, err := time.Parse(time.RFC3339, rawDate); err == nil {
				displayDate = t.Format("2006-01-02")
			}
			waterStr := "-"
			if water.Valid {
				waterStr = fmt.Sprintf("%.1fL", water.Float64)
				sumWater += water.Float64
				waterN++
			}
			fmt.Printf("%s  sleep:%.1fh  quality:%.1f  feel:%.1f  water:%s\n", displayDate, hours, quality, feel, waterStr)

			sumHours += hours
			sumQuality += quality
			sumFeel += feel
			n++
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if n == 0 {
			fmt.Println("No checkins logged this week.")
			return nil
		}

		avgWaterStr := "-"
		if waterN > 0 {
			avgWaterStr = fmt.Sprintf("%.1fL", sumWater/float64(waterN))
		}
		fmt.Printf("\nAvg sleep: %.1fh | Avg quality: %.1f | Avg feel: %.1f | Avg water: %s\n",
			sumHours/float64(n), sumQuality/float64(n), sumFeel/float64(n), avgWaterStr)

		return nil
	},
}

var metricsCorrelateCmd = &cobra.Command{
	Use:   "correlate",
	Short: "Correlate sleep hours/quality/feel with average daily focus quality",
	RunE: func(cmd *cobra.Command, args []string) error {
		rows, err := conn.Query(`
			SELECT c.sleep_hours, c.sleep_quality, c.feel, AVG(b.focus_quality)
			FROM daily_checkin c
			JOIN blocks b ON b.date = c.date AND b.focus_quality IS NOT NULL
			GROUP BY c.date
			ORDER BY c.date`)
		if err != nil {
			return err
		}
		defer rows.Close()

		var hours, quality, feel, focus []float64
		for rows.Next() {
			var h, avgFocus, q, f float64
			if err := rows.Scan(&h, &q, &f, &avgFocus); err != nil {
				return err
			}
			hours = append(hours, h)
			quality = append(quality, q)
			feel = append(feel, f)
			focus = append(focus, avgFocus)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if len(hours) < 3 {
			fmt.Printf("Only %d paired days found. Need at least 3 (ideally 14+) for a meaningful correlation.\n", len(hours))
			return nil
		}

		fmt.Printf("Paired days: %d\n", len(hours))
		fmt.Printf("sleep_hours   <-> focus_quality  r=%.2f\n", pearson(hours, focus))
		fmt.Printf("sleep_quality <-> focus_quality  r=%.2f\n", pearson(quality, focus))
		fmt.Printf("feel          <-> focus_quality  r=%.2f\n", pearson(feel, focus))
		fmt.Println("\n(|r|<0.3 weak, 0.3-0.6 moderate, >0.6 strong — treat as trend, not proof, until 30+ days)")
		return nil
	},
}

// pearson computes the Pearson correlation coefficient between two equal-length series.
func pearson(xs, ys []float64) float64 {
	n := float64(len(xs))
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
		sumY2 += ys[i] * ys[i]
	}
	den := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / den
}

func init() {
	metricsCmd.AddCommand(metricsWeekCmd, metricsSleepCmd, metricsCorrelateCmd)
	rootCmd.AddCommand(metricsCmd)
}
