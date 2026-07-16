package site

import (
	"fmt"
	"os"
	"time"
)

func BackupUsage() {
	fmt.Fprint(os.Stderr, `  site-backup --prefix=<path> --name=<site>
        commit the container to a timestamped image and archive its files
`)
}

func Backup(args []string) error { return fail("site-backup", backup(args)) }

func backup(args []string) error {
	o := options("site-backup")
	o.fs.Usage = BackupUsage
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

	stamp := time.Now().Format("20060102-150405")
	id := name + "-" + stamp
	img := container(name) + ":" + stamp

	if err := docker("commit", container(name), img); err != nil {
		return err
	}
	if err := archive(root, name, backupPath(root, id, ".tgz")); err != nil {
		return err
	}
	if err := writeManifest(root, manifest{ID: id, Image: img, Config: c}); err != nil {
		return err
	}

	fmt.Printf("backed up %s\n\n", name)
	fmt.Printf("  backupID  %s\n", id)
	fmt.Printf("  image     %s\n", img)
	fmt.Printf("  files     %s\n", backupPath(root, id, ".tgz"))
	return nil
}
