package report

import (
	"os"
)

func writeFileString(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
