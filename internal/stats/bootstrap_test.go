package stats

import (
	"math"
	"math/rand"
	"testing"

	"can-i-compress-it/internal/domain"
)

func TestAccuracy(t *testing.T) {
	rs := []domain.CellResult{{SelectionCorrect: true, ParamFillCorrect: true}, {SelectionCorrect: true}}
	if got := Accuracy(rs); got != 0.5 {
		t.Fatalf("got %v want 0.5", got)
	}
	if Accuracy(nil) != 0 {
		t.Fatal("empty results must be zero")
	}
}

func TestBootstrapDropCIPerfectDrop(t *testing.T) {
	base := []float64{1, 1, 1, 1, 1, 0, 0, 0, 0, 0}
	lvl := make([]float64, len(base))
	for i := range lvl {
		lvl[i] = base[i]
	}
	rng := rand.New(rand.NewSource(1))
	low, high := BootstrapDropCI(base, lvl, 500, rng)
	if low != 0 || high != 0 {
		t.Fatalf("identical vectors should give zero CI, got [%v,%v]", low, high)
	}

	worse := []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	low, high = BootstrapDropCI(base, worse, 500, rng)
	if low < 0.49 || high > 0.51 {
		t.Fatalf("expected ~0.5 drop CI, got [%v,%v]", low, high)
	}
}

func TestDetectCliff(t *testing.T) {
	scores := map[domain.CompressionLevel]float64{
		domain.LevelNone:   0.9,
		domain.LevelLow:    0.88,
		domain.LevelMedium: 0.85,
		domain.LevelHigh:   0.70,
		domain.LevelMax:    0.30,
	}
	cliff, drop, found := DetectCliff(domain.AllLevels, scores)
	if !found || cliff != domain.LevelHigh {
		t.Fatalf("got cliff=%v drop=%v found=%v", cliff, drop, found)
	}
	if diff := drop - 0.20; diff*diff > 1e-9 {
		t.Fatalf("expected drop 0.20, got %v", drop)
	}
}

func TestLogRegressionExactLine(t *testing.T) {
	points := [][2]float64{{1, 2}, {math.E, 4}, {math.E * math.E, 6}}
	alpha, beta, r2 := LogRegression(points)
	if diff := (alpha - 2) + (beta - 2); diff*diff > 1e-9 {
		t.Fatalf("bad fit alpha=%v beta=%v", alpha, beta)
	}
	if r2 < 0.9999 {
		t.Fatalf("R^2 should be ~1, got %v", r2)
	}
}
