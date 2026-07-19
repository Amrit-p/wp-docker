package site

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wpdock/src/commands/db"
	"wpdock/src/prefix"
	"wpdock/src/prompt"
)

func ExtractUsage() {
	fmt.Fprint(os.Stderr, `  site-extract --prefix=<path> --name=<site> --zip_path=<zip> --sql_path=<sql> [--force=y]
        replace a site's files and database from a zip and a sql dump
`)
}

func Extract(args []string) error { return fail("site-extract", extractSite(args)) }

func extractSite(args []string) error {
	o := options("site-extract")
	o.fs.Usage = ExtractUsage
	zipPath := o.fs.String("zip_path", "", "zip whose contents replace the site's files")
	sqlPath := o.fs.String("sql_path", "", "sql dump loaded into the site's database")
	force := o.fs.String("force", "", "y to skip the confirmation prompt")
	if ok, err := parse(o.fs, args); !ok {
		return err
	}

	root, err := o.root()
	if err != nil {
		return err
	}
	if err := o.cfg.checkName(); err != nil {
		return err
	}
	name := o.cfg.Name

	c, err := inspect(name)
	if err != nil {
		return err
	}

	if *zipPath == "" {
		return fmt.Errorf("--zip_path is required")
	}
	zip, err := prefix.Resolve(*zipPath)
	if err != nil {
		return err
	}
	if err := readable("--zip_path", zip); err != nil {
		return err
	}

	var sql string
	if c.DBName != "" {
		if *sqlPath == "" {
			return fmt.Errorf("--sql_path is required for %s, which has a database", name)
		}
		if sql, err = prefix.Resolve(*sqlPath); err != nil {
			return err
		}
		if err := readable("--sql_path", sql); err != nil {
			return err
		}
	} else if *sqlPath != "" {
		fmt.Printf("note: %s has no database; ignoring --sql_path\n", name)
	}

	fmt.Printf("extract into %s\n\n", name)
	field("files", fmt.Sprintf("%s -> %s", zip, dataDir(root, name)))
	if c.DBName != "" {
		field("database", fmt.Sprintf("%s -> %s@%s", sql, c.DBName, c.DBHost))
	}
	fmt.Printf("\n  ! this overwrites the site's current files%s\n\n", dbAlso(c))

	if !forced(*force) {
		ok, err := prompt.Confirm("replace them?")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cancelled")
		}
	}

	if c.DBName != "" {
		if err := db.TruncateAll(c.DBHost, c.DBName, c.DBUser, c.DBPass); err != nil {
			return err
		}
		if err := db.ImportSQL(c.DBHost, c.DBName, c.DBUser, c.DBPass, sql); err != nil {
			return err
		}
	}

	if err := unpackZip(root, name, zip); err != nil {
		return err
	}
	if err := docker("restart", container(name)); err != nil {
		return err
	}

	fmt.Printf("\nextracted into %s\n", name)
	return nil
}

func readable(flag, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %v", flag, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s: %s: is a directory", flag, path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s: %s: is empty", flag, path)
	}
	return nil
}

func forced(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "y", "yes":
		return true
	}
	return false
}

func dbAlso(c *Config) string {
	if c.DBName != "" {
		return " and database"
	}
	return ""
}

func unpackZip(root, name, in string) error {
	return helper(root, []string{"-v", filepath.Dir(in) + ":/in:ro"},
		fmt.Sprintf("mkdir -p /data/%[1]s && find /data/%[1]s -mindepth 1 -delete && unzip -q -o /in/%[2]s -d /data/%[1]s && chown -R 33:33 /data/%[1]s",
			name, filepath.Base(in)))
}
