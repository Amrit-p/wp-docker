package site

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"wpdock/src/prefix"
)

func ShellUsage() {
	fmt.Fprint(os.Stderr, `  site-shell --prefix=<path> --name=<site> --user=<u> --password=<p> --port=<n>
        open a shell inside one site's container for a developer
  site-shell --prefix=<path> --name=<site> --close
        remove that shell access
`)
}

func Shell(args []string) error { return fail("site-shell", shell(args)) }

func shell(args []string) error {
	fs := flag.NewFlagSet("site-shell", flag.ContinueOnError)
	dir := fs.String("prefix", ".", "directory wpdock was installed into (default: current directory)")
	name := fs.String("name", "", "site to grant access to")
	user := fs.String("user", "", "shell username for the developer")
	password := fs.String("password", "", "shell password for the developer")
	port := fs.Int("port", 0, "host port to expose ssh on")
	closed := fs.Bool("close", false, "remove shell access instead of granting it")
	fs.Usage = ShellUsage
	if ok, err := parse(fs, args); !ok {
		return err
	}

	if err := (&Config{Name: *name}).checkName(); err != nil {
		return err
	}

	root, err := prefix.Resolve(*dir)
	if err != nil {
		return err
	}

	c, err := inspect(*name)
	if err != nil {
		return err
	}

	if *closed {
		if c.SSHPort == "" {
			return fmt.Errorf("no shell access is open for %s", *name)
		}
		c.SSHPort, c.SSHUser, c.SSHPass = "", "", ""
	} else {
		if !nameRe.MatchString(*user) {
			return fmt.Errorf("--user: %q: use letters, digits, - and _", *user)
		}
		if *password == "" {
			return fmt.Errorf("--password is required")
		}
		if strings.ContainsRune(*password, '\n') {
			return fmt.Errorf("--password: must not contain a newline")
		}
		if *port < 1 || *port > 65535 {
			return fmt.Errorf("--port is required (1-65535)")
		}
		c.SSHPort, c.SSHUser, c.SSHPass = strconv.Itoa(*port), *user, *password
	}

	if err := ensurePortFree(c.SSHPort, container(*name)); err != nil {
		return err
	}

	img, err := prepareImage(c)
	if err != nil {
		return err
	}

	if err := docker("rm", "-f", container(*name)); err != nil {
		return err
	}
	if err := run(root, c, img); err != nil {
		return err
	}
	if err := publish(root, c); err != nil {
		return err
	}

	if *closed {
		fmt.Printf("closed shell for %s\n", *name)
		return nil
	}

	fmt.Printf("shell open for %s\n\n", *name)
	field("host port", c.SSHPort)
	field("user", c.SSHUser)
	fmt.Printf("\nconnect:  ssh -p %s %s@<server>\n", c.SSHPort, c.SSHUser)
	fmt.Println("files:    /var/www/html   (the developer has sudo inside the container)")
	return nil
}
