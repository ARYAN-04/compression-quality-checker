package report

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	got, err := cleanPath("~")
	if err != nil || got != home {
		t.Fatalf("cleanPath(~) = (%q, %v), want (%q, nil)", got, err, home)
	}

	want := filepath.Join(home, "results", "report.html")
	got, err = cleanPath("~/results/report.html")
	if err != nil || got != want {
		t.Fatalf("cleanPath(~/results/report.html) = (%q, %v), want (%q, nil)", got, err, want)
	}

	got, err = cleanPath(`~\results\report.html`)
	if err != nil || got != want {
		t.Fatalf("cleanPath(~\\results\\report.html) = (%q, %v), want (%q, nil)", got, err, want)
	}
}
