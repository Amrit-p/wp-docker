package db

import (
	"fmt"
	"strings"

	"wpdock/src/prompt"
)

func truncate(container, name, user, password string, yes bool) error {
	if err := checkIdent("--db_name", name, 64); err != nil {
		return err
	}
	if err := checkIdent("--db_user", user, 80); err != nil {
		return err
	}

	as := login{container: container, user: user, password: password}

	rows, err := as.query(name, fmt.Sprintf("SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = %s ORDER BY table_name;", quoteString(name)))
	if err != nil {
		return err
	}

	tables, views := split(rows)

	fmt.Printf("db %s\n\n", container)
	fmt.Printf("  database  %s\n", name)
	fmt.Printf("  user      %s@%s\n", user, host)

	if len(tables)+len(views) == 0 {
		fmt.Printf("  empty     nothing to drop\n")
		return nil
	}

	fmt.Printf("  drop      %s\n\n", counts(tables, views))
	for _, t := range append(append([]string{}, views...), tables...) {
		fmt.Printf("    %s\n", t)
	}
	fmt.Println()

	if !yes {
		ok, err := prompt.Confirm("drop them?")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cancelled")
		}
	}

	if err := as.exec(strings.NewReader(drops(tables, views)), nil, name); err != nil {
		return err
	}

	fmt.Printf("\nemptied %s, ready to import into\n", name)
	return nil
}

func split(rows []string) (tables, views []string) {
	for _, row := range rows {
		name, kind, found := strings.Cut(row, "\t")
		if !found {
			continue
		}
		if kind == "VIEW" {
			views = append(views, name)
			continue
		}
		tables = append(tables, name)
	}
	return tables, views
}

func counts(tables, views []string) string {
	out := []string{plural(len(tables), "table")}
	if len(views) > 0 {
		out = append(out, plural(len(views), "view"))
	}
	return strings.Join(out, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func drops(tables, views []string) string {
	out := []string{"SET FOREIGN_KEY_CHECKS = 0;"}

	if len(views) > 0 {
		out = append(out, fmt.Sprintf("DROP VIEW IF EXISTS %s;", list(views)))
	}
	if len(tables) > 0 {
		out = append(out, fmt.Sprintf("DROP TABLE IF EXISTS %s;", list(tables)))
	}

	out = append(out, "SET FOREIGN_KEY_CHECKS = 1;", "")
	return strings.Join(out, "\n")
}

func list(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, quoteIdent(n))
	}
	return strings.Join(quoted, ", ")
}
