package site

import (
	"fmt"
	"os"
)

func StopUsage() {
	fmt.Fprint(os.Stderr, `  site-stop [--prefix=<path>] --name=<site>
        stop the site's container, leaving its files and database in place
`)
}

func Stop(args []string) error { return fail("site-stop", stop(args)) }

func stop(args []string) error {
	o := options("site-stop")
	o.fs.Usage = StopUsage
	if ok, err := parse(o.fs, args); !ok {
		return err
	}

	if err := o.cfg.checkName(); err != nil {
		return err
	}
	if err := docker("stop", container(o.cfg.Name)); err != nil {
		return err
	}

	fmt.Printf("stopped %s\n", o.cfg.Name)
	return nil
}
