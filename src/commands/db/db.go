package db

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func Usage() {
	fmt.Fprint(os.Stderr, `  db --create-user --db_container=<name> --db_name=<db> --db_user=<user> --db_password=<pass> --root_password=<pass>
        create a mariadb user inside <container> that may reach only <db>
  db --import --db_container=<name> --db_name=<db> --db_user=<user> --db_password=<pass> --sql_file=<path>
        load <path> into <db>, as <user>, so it may touch nothing else
  db --truncate --db_container=<name> --db_name=<db> --db_user=<user> --db_password=<pass> [--yes]
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
	create := fs.Bool("create-user", false, "create a user that may reach only one database")
	load := fs.Bool("import", false, "load a sql file into one database")
	empty := fs.Bool("truncate", false, "drop every table in one database")
	container := fs.String("db_container", "", "name of the running mariadb container")
	name := fs.String("db_name", "", "database to act on")
	user := fs.String("db_user", "", "user to create, or to act as")
	password := fs.String("db_password", "", "password of that user")
	root := fs.String("root_password", "", "password of the mariadb root user, which creates the user")
	file := fs.String("sql_file", "", "path to the sql file to import")
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
			{"--db_container", *container},
			{"--db_name", *name},
			{"--db_user", *user},
			{"--db_password", *password},
			{"--root_password", *root},
		}); err != nil {
			return err
		}
		if err := createUser(*container, *name, *user, *password, *root); err != nil {
			return fmt.Errorf("db: %v", err)
		}

	case *load:
		if err := required([]flags{
			{"--db_container", *container},
			{"--db_name", *name},
			{"--db_user", *user},
			{"--db_password", *password},
			{"--sql_file", *file},
		}); err != nil {
			return err
		}
		if err := importSQL(*container, *name, *user, *password, *file); err != nil {
			return fmt.Errorf("db: %v", err)
		}

	case *empty:
		if err := required([]flags{
			{"--db_container", *container},
			{"--db_name", *name},
			{"--db_user", *user},
			{"--db_password", *password},
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
