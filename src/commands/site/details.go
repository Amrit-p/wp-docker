package site

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func DetailsUsage() {
	fmt.Fprint(os.Stderr, `  site-details [--prefix=<path>] --name=<site>
        show one site's settings, image and state
`)
}

func Details(args []string) error { return fail("site-details", details(args)) }

func details(args []string) error {
	fs := flag.NewFlagSet("site-details", flag.ContinueOnError)
	name := fs.String("name", "", "site name")
	fs.String("prefix", "", "installation directory (accepted for consistency; site-details does not use it)")
	fs.Usage = DetailsUsage
	if ok, err := parse(fs, args); !ok {
		return err
	}

	c := &Config{Name: *name}
	if err := c.checkName(); err != nil {
		return err
	}

	c, err := inspect(*name)
	if err != nil {
		return err
	}

	status, image, err := runtime(*name)
	if err != nil {
		return err
	}

	describeCore(c)
	if c.DBName != "" {
		field("db-host", c.DBHost)
		field("db-name", c.DBName)
		field("db-user", c.DBUser)
		field("db-password", c.DBPass)
	}
	field("image", image)
	field("status", status)
	if c.SSHPort != "" {
		field("shell", fmt.Sprintf("open (port %s, user %s)", c.SSHPort, c.SSHUser))
	} else {
		field("shell", "closed")
	}
	return nil
}

func runtime(name string) (status, image string, err error) {
	out, err := dockerOut("inspect", "-f", "{{.State.Status}}\t{{.Config.Image}}", container(name))
	if err != nil {
		return "", "", err
	}

	f := strings.SplitN(strings.TrimSpace(out), "\t", 2)
	if len(f) < 2 {
		return f[0], "", nil
	}
	return f[0], f[1], nil
}
