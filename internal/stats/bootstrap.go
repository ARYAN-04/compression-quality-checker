// Package stats computes bootstrap confidence intervals, failure cliffs,
// and the log-scale parameter regression for safety guidance.
package stats

import (
	"math"
	"math/rand"
	"sort"

	"can-i-compress-it/internal/domain"
)

// Accuracy returns the proportion of results where both tool selection
// and parameter fill were correct.
func Accuracy(results []domain.CellResult) float64 {
	if len(results) == 0 {
		return 0
	}
	correct := 0
	for _, r := range results {
		if r.Correct() {
			correct++
		}
	}
	return float64(correct) / float64(len(results))
}

// BootstrapDropCI computes the 95% CI of the accuracy drop between an
// uncompressed baseline vector and one compressed-level vector using
// paired resampling over shared task indices.
func BootstrapDropCI(baseline, level []float64, iterations int, rng *rand.Rand) (low, high float64) {
	n := min(len(baseline), len(level))
	if n == 0 || iterations <= 0 {
		return 0, 0
	}
	deltas := make([]float64, iterations)
	for i := range deltas {
		var sum float64
		for _, j := range rng.Perm(n)[:n] {
			sum += baseline[j] - level[j]
		}
		deltas[i] = sum / float64(n)
	}
	sort.Float64s(deltas)
	return quantile(deltas, 0.025), quantile(deltas, 0.975)
}

func quantile(sorted []float64, q float64) float64 {
	i := int(q * float64(len(sorted)-1))
	return sorted[i]
}

// DetectCliff finds the first level whose accuracy drops >= 10 percentage
// points below the uncompressed baseline.
func DetectCliff(levels []domain.CompressionLevel, scores map[domain.CompressionLevel]float64) (cliff domain.CompressionLevel, drop float64, found bool) {
	baseline := scores[domain.LevelNone]
	for _, l := range levels {
		if l == domain.LevelNone {
			continue
		}
		d := baseline - scores[l]
		if d >= 0.10 {
			return l, d, true
		}
	}
	return "", math.NaN(), false
}

// LogRegression fits y = alpha + beta*ln(paramCountB) by ordinary least
// squares and reports beta with R^2.
func LogRegression(points [][2]float64) (alpha, beta, r2 float64) {
	if len(points) < 2 {
		return math.NaN(), math.NaN(), 0
	}
	var sx, sy, sxx, sxy float64
	n := float64(len(points))
	for _, p := range points {
		x := math.Log(p[0])
		sx += x
		sy += p[1]
		sxx += x * x
		sxy += x * p[1]
	}
	denom := n*sxx - sx*sx
	if denom == 0 {
		return math.NaN(), math.NaN(), 0
	}
	beta = (n*sxy - sx*sy) / denom
	alpha = (sy - beta*sx) / n

	meanY := sy / n
	var ssTot, ssRes float64
	for _, p := range points {
		pred := alpha + beta*math.Log(p[0])
		ssRes += (p[1] - pred) * (p[1] - pred)
		ssTot += (p[1] - meanY) * (p[1] - meanY)
	}
	if ssTot != 0 {
		r2 = 1 - ssRes/ssTot
	}
	return alpha, beta, r2
}
