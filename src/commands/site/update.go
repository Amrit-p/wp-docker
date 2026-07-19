package site

import (
	"fmt"
	"os"
)

func UpdateUsage() {
	fmt.Fprint(os.Stderr, `  site-update --prefix=<path> --name=<site> [flags to change]
        recreate a site's container with changed flags, keeping its files
`)
}

func Update(args []string) error { return fail("site-update", update(args)) }

func update(args []string) error {
	o := options("site-update")
	o.fs.Usage = UpdateUsage
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

	c, err := inspect(o.cfg.Name)
	if err != nil {
		return err
	}
	merge(c, o)

	if err := c.validate(); err != nil {
		return err
	}

	if err := ensurePortFree(c.SSHPort, container(c.Name)); err != nil {
		return err
	}

	img, err := prepareImage(c)
	if err != nil {
		return err
	}

	if err := docker("rm", "-f", container(c.Name)); err != nil {
		return err
	}
	if err := run(root, c, img); err != nil {
		return err
	}
	if err := publish(root, c); err != nil {
		return err
	}

	describe(c)
	fmt.Printf("\nupdated %s\n", c.Name)
	return nil
}

func merge(base *Config, o *opts) {
	for _, f := range []struct {
		flag string
		set  func()
	}{
		{"domain", func() { base.Domain = o.cfg.Domain }},
		{"aliases", func() { base.Aliases = o.cfg.Aliases }},
		{"type", func() { base.Type = o.cfg.Type }},
		{"runtime", func() { base.Runtime = o.cfg.Runtime }},
		{"wp-version", func() { base.Version = o.cfg.Version }},
		{"php-version", func() { base.PHP = o.cfg.PHP }},
		{"memory", func() { base.Memory = o.cfg.Memory }},
		{"cpu", func() { base.CPU = o.cfg.CPU }},
		{"pids", func() { base.PIDs = o.cfg.PIDs }},
		{"db-host", func() { base.DBHost = o.cfg.DBHost }},
		{"db-name", func() { base.DBName = o.cfg.DBName }},
		{"db-user", func() { base.DBUser = o.cfg.DBUser }},
		{"db-password", func() { base.DBPass = o.cfg.DBPass }},
	} {
		if o.set(f.flag) {
			f.set()
		}
	}
}
