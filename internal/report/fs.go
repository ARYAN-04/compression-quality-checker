package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cleanPath(p string) (string, error) {
	if p == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("user home dir: %w", err)
		}
		rel := filepath.FromSlash(strings.ReplaceAll(p[2:], `\`, "/"))
		p = filepath.Join(home, rel)
	}
	dir := filepath.Dir(p)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create parent directory %s: %w", dir, err)
		}
	}
	return p, nil
}

func writeFileString(path, content string) error {
	clean, err := cleanPath(path)
	if err != nil {
		return err
	}
	return os.WriteFile(clean, []byte(content), 0o644)
}
