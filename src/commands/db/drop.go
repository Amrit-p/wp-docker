package db

import (
	"fmt"
	"strings"
)

func DropDatabase(container, name, user, password string) error {
	if err := checkIdent("--db-name", name, 64); err != nil {
		return err
	}
	if err := checkIdent("--db-user", user, 80); err != nil {
		return err
	}

	as := login{container: container, user: user, password: password}

	sql := fmt.Sprintf("DROP DATABASE IF EXISTS %s;\n", quoteIdent(name))
	return as.exec(strings.NewReader(sql), nil)
}
