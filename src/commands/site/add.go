package site

import (
	"fmt"
	"os"
)

func AddUsage() {
	fmt.Fprint(os.Stderr, `  site-add --prefix=<path> --name=<site> --domain=<domain> --type=<wordpress|php> --php-version=<v> [flags]
        create and start a site container, then route <domain> to it
`)
}

func Add(args []string) error { return fail("site-add", add(args)) }

func add(args []string) error {
	o := options("site-add")
	o.fs.Usage = AddUsage
	if ok, err := parse(o.fs, args); !ok {
		return err
	}

	root, err := o.root()
	if err != nil {
		return err
	}

	c := o.cfg
	if err := c.validate(); err != nil {
		return err
	}

	if siteExists(c.Name) {
		return fmt.Errorf("a site named %q already exists — use site-update to change it, or site-nuke to remove it", c.Name)
	}

	if err := ensurePortFree(c.SSHPort, container(c.Name)); err != nil {
		return err
	}

	img, err := prepareImage(c)
	if err != nil {
		return err
	}

	if err := ensureNetwork(); err != nil {
		return err
	}
	if err := run(root, c, img); err != nil {
		return err
	}
	if err := publish(root, c); err != nil {
		return err
	}

	describe(c)
	fmt.Printf("\nadded %s, routing %s\n", c.Name, serverName(c))
	return nil
}
