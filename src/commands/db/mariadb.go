package db

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const host = "%"

const client = `if command -v mariadb >/dev/null 2>&1; then bin=mariadb; else bin=mysql; fi
user=$1
shift
exec "$bin" --protocol=socket --default-character-set=utf8mb4 -u"$user" "$@"`

var ident = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type login struct {
	container string
	user      string
	password  string
}

func (l login) exec(sql io.Reader, out io.Writer, args ...string) error {
	argv := append([]string{"exec", "-i", "-e", "MYSQL_PWD", l.container, "sh", "-c", client, "sh", l.user}, args...)

	cmd := exec.Command("docker", argv...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+l.password)
	cmd.Stdin = sql

	var errs bytes.Buffer
	cmd.Stdout = &errs
	cmd.Stderr = &errs
	if out != nil {
		cmd.Stdout = out
	}

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errs.String()); msg != "" {
			return fmt.Errorf("%s: %v: %s", l.container, err, msg)
		}
		return fmt.Errorf("%s: %v", l.container, err)
	}

	return nil
}

func (l login) query(database, sql string) ([]string, error) {
	var out bytes.Buffer

	if err := l.exec(strings.NewReader(sql), &out, "-N", "-B", database); err != nil {
		return nil, err
	}

	rows := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(rows) == 1 && rows[0] == "" {
		return nil, nil
	}

	return rows, nil
}

func checkIdent(flag, value string, max int) error {
	if !ident.MatchString(value) {
		return fmt.Errorf("%s: %q: only letters, digits and underscores", flag, value)
	}
	if len(value) > max {
		return fmt.Errorf("%s: %q: longer than %d characters", flag, value, max)
	}
	return nil
}

func quoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

func quoteString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(s) + "'"
}
