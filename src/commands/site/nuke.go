package site

import (
	"fmt"
	"os"

	"wpdock/src/commands/db"
	"wpdock/src/prompt"
)

func NukeUsage() {
	fmt.Fprint(os.Stderr, `  site-nuke --prefix=<path> --name=<site> [--yes]
        delete the site's container, files and database
`)
}

func Nuke(args []string) error { return fail("site-nuke", nuke(args)) }

func nuke(args []string) error {
	o := options("site-nuke")
	o.fs.Usage = NukeUsage
	yes := o.fs.Bool("yes", false, "skip the confirmation prompt")
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

	fmt.Printf("nuke %s\n\n", name)
	field("container", container(name))
	field("files", dataDir(root, name))
	if c.DBName != "" {
		field("database", fmt.Sprintf("%s@%s", c.DBName, c.DBHost))
	}
	fmt.Println()

	if !*yes {
		ok, err := prompt.Confirm("delete all of this?")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cancelled")
		}
	}

	if err := docker("rm", "-f", container(name)); err != nil {
		return err
	}
	if err := removeVhost(root, name); err != nil {
		return err
	}
	if err := reload(); err != nil {
		return err
	}
	if err := removeData(root, name); err != nil {
		return err
	}
	if c.DBName != "" {
		if err := db.DropDatabase(c.DBHost, c.DBName, c.DBUser, c.DBPass); err != nil {
			return err
		}
	}

	fmt.Printf("\nnuked %s\n", name)
	return nil
}
