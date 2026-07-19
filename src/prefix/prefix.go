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

func Project(root string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(filepath.Base(root)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	out := strings.TrimLeft(b.String(), "-_")
	if out == "" {
		return "wpdock"
	}
	return out
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
