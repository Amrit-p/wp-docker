package site

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func ListUsage() {
	fmt.Fprint(os.Stderr, `  site-list [--prefix=<path>]
        list every wpdock-managed site and its state
`)
}

func List(args []string) error { return fail("site-list", list(args)) }

func list(args []string) error {
	fs := flag.NewFlagSet("site-list", flag.ContinueOnError)
	fs.Usage = ListUsage
	fs.String("prefix", "", "installation directory (accepted for consistency; site-list does not use it)")
	if ok, err := parse(fs, args); !ok {
		return err
	}

	out, err := dockerOut("ps", "-a",
		"--filter", "label="+labelPrefix+"managed=true",
		"--format", `table {{.Names}}\t{{.State}}\t{{.Label "wpdock.type"}}\t{{.Label "wpdock.domain"}}`)
	if err != nil {
		return err
	}

	out = strings.TrimRight(out, "\n")
	if len(strings.Split(out, "\n")) < 2 {
		fmt.Println("no sites")
		return nil
	}

	fmt.Println(out)
	return nil
}
