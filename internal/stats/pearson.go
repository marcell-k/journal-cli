package stats

import "math"

// Pearson computes the Pearson correlation coefficient between two equal-length series.
func Pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) {
		return 0
	}
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
