package site

import (
	"fmt"
	"os"
)

func RestoreUsage() {
	fmt.Fprint(os.Stderr, `  site-restore --prefix=<path> --backupID=<id>
        recreate a site from a backup: its image, files and routing
`)
}

func Restore(args []string) error { return fail("site-restore", restore(args)) }

func restore(args []string) error {
	o := options("site-restore")
	o.fs.Usage = RestoreUsage
	id := o.fs.String("backupID", "", "the backup to restore, e.g. blog-20260715-203000")
	if ok, err := parse(o.fs, args); !ok {
		return err
	}

	root, err := o.root()
	if err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--backupID is required")
	}

	m, err := readManifest(root, *id)
	if err != nil {
		return err
	}
	c := m.Config

	if err := ensurePortFree(c.SSHPort, container(c.Name)); err != nil {
		return err
	}

	if err := extract(root, c.Name, backupPath(root, *id, ".tgz")); err != nil {
		return err
	}

	_ = docker("rm", "-f", container(c.Name))

	if err := ensureNetwork(); err != nil {
		return err
	}
	if err := run(root, c, m.Image); err != nil {
		return err
	}
	if err := publish(root, c); err != nil {
		return err
	}

	describe(c)
	fmt.Printf("\nrestored %s from %s\n", c.Name, *id)
	return nil
}
