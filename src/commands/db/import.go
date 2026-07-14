package db

import (
	"fmt"
	"os"
)

func importSQL(container, name, user, password, path string) error {
	if err := checkIdent("--db_name", name, 64); err != nil {
		return err
	}
	if err := checkIdent("--db_user", user, 80); err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("--sql_file: %v", err)
	}
	if info.IsDir() {
		return fmt.Errorf("--sql_file: %s: is a directory", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("--sql_file: %s: is empty", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("--sql_file: %v", err)
	}
	defer f.Close()

	as := login{container: container, user: user, password: password}

	if err := as.exec(f, nil, name); err != nil {
		return err
	}

	fmt.Printf("db %s\n\n", container)
	fmt.Printf("  database  %s\n", name)
	fmt.Printf("  user      %s@%s\n", user, host)
	fmt.Printf("  imported  %s (%s)\n", path, size(info.Size()))
	return nil
}

func size(n int64) string {
	const unit = 1024

	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for n/div >= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
