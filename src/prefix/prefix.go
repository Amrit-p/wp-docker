package prefix

import (
	"os"
	"path/filepath"
	"strings"
)

func Resolve(path string) (string, error) {
	expanded, err := expandTilde(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}

func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
