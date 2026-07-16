package db

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func Usage() {
	fmt.Fprint(os.Stderr, `  db --create-user --db-container=<name> --db-name=<db> --db-user=<user> --db-password=<pass> --root-password=<pass>
        create a mariadb user inside <container> that may reach only <db>
  db --import --db-container=<name> --db-name=<db> --db-user=<user> --db-password=<pass> --sql-file=<path>
        load <path> into <db>, as <user>, so it may touch nothing else
  db --truncate --db-container=<name> --db-name=<db> --db-user=<user> --db-password=<pass> [--yes]
        drop every table in <db>, leaving it empty and ready to import into
`)
}

type flags struct {
	flag  string
	value string
}

func Run(args []string) error {
	fs := flag.NewFlagSet("db", flag.ContinueOnError)
	fs.Usage = Usage
	fs.String("prefix", "", "installation directory (accepted for consistency; db does not use it)")
	create := fs.Bool("create-user", false, "create a user that may reach only one database")
	load := fs.Bool("import", false, "load a sql file into one database")
	empty := fs.Bool("truncate", false, "drop every table in one database")
	container := fs.String("db-container", "", "name of the running mariadb container")
	name := fs.String("db-name", "", "database to act on")
	user := fs.String("db-user", "", "user to create, or to act as")
	password := fs.String("db-password", "", "password of that user")
	root := fs.String("root-password", "", "password of the mariadb root user, which creates the user")
	file := fs.String("sql-file", "", "path to the sql file to import")
	yes := fs.Bool("yes", false, "skip the confirmation prompt --truncate asks")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	var picked []string
	for _, op := range []struct {
		flag string
		on   bool
	}{
		{"--create-user", *create},
		{"--import", *load},
		{"--truncate", *empty},
	} {
		if op.on {
			picked = append(picked, op.flag)
		}
	}

	if len(picked) > 1 {
		return fmt.Errorf("db: %s do different things, so pass one of them", strings.Join(picked, " and "))
	}

	switch {
	case *create:
		if err := required([]flags{
			{"--db-container", *container},
			{"--db-name", *name},
			{"--db-user", *user},
			{"--db-password", *password},
			{"--root-password", *root},
		}); err != nil {
			return err
		}
		if err := CreateUser(*container, *name, *user, *password, *root); err != nil {
			return fmt.Errorf("db: %v", err)
		}

	case *load:
		if err := required([]flags{
			{"--db-container", *container},
			{"--db-name", *name},
			{"--db-user", *user},
			{"--db-password", *password},
			{"--sql-file", *file},
		}); err != nil {
			return err
		}
		if err := ImportSQL(*container, *name, *user, *password, *file); err != nil {
			return fmt.Errorf("db: %v", err)
		}

	case *empty:
		if err := required([]flags{
			{"--db-container", *container},
			{"--db-name", *name},
			{"--db-user", *user},
			{"--db-password", *password},
		}); err != nil {
			return err
		}
		if err := truncate(*container, *name, *user, *password, *yes); err != nil {
			return fmt.Errorf("db: %v", err)
		}

	default:
		return fmt.Errorf("db: one of --create-user, --import or --truncate is required")
	}

	return nil
}

func required(fs []flags) error {
	for _, f := range fs {
		if f.value == "" {
			return fmt.Errorf("db: %s is required", f.flag)
		}
	}
	return nil
}
