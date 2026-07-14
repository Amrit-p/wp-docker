package db

import (
	"fmt"
	"strings"
)

func createUser(container, name, user, password, root string) error {
	if err := checkIdent("--db_name", name, 64); err != nil {
		return err
	}
	if err := checkIdent("--db_user", user, 80); err != nil {
		return err
	}
	if strings.ContainsRune(password, 0) {
		return fmt.Errorf("--db_password: must not contain a null byte")
	}

	as := login{container: container, user: "root", password: root}

	if err := as.exec(strings.NewReader(statements(name, user, password)), nil); err != nil {
		return err
	}

	fmt.Printf("db %s\n\n", container)
	fmt.Printf("  database  %s\n", name)
	fmt.Printf("  user      %s@%s\n", user, host)
	fmt.Printf("  grants    all privileges on %s.* and nothing else\n", name)
	return nil
}

func statements(name, user, password string) string {
	db := quoteIdent(name)
	who := quoteString(user) + "@" + quoteString(host)
	secret := quoteString(password)

	return strings.Join([]string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", db),
		fmt.Sprintf("CREATE USER IF NOT EXISTS %s IDENTIFIED BY %s;", who, secret),
		fmt.Sprintf("ALTER USER %s IDENTIFIED BY %s;", who, secret),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON %s.* TO %s;", db, who),
		"FLUSH PRIVILEGES;",
		"",
	}, "\n")
}
