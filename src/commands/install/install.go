package install

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"wpdock/src/prefix"
)

func Usage() {
	fmt.Fprint(os.Stderr, `  install --prefix=<path> [--force] [--yes]
        create the wpdock tree (sites.json, data/, nginx/, www/) at <path>
`)
}

func Run(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.Usage = Usage
	dir := fs.String("prefix", "", "directory to install into")
	force := fs.Bool("force", false, "rewrite generated files that already exist")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *dir == "" {
		return fmt.Errorf("install: --prefix is required")
	}

	root, err := prefix.Resolve(*dir)
	if err != nil {
		return fmt.Errorf("install: %s: %v", *dir, err)
	}

	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		return fmt.Errorf("install: %s: not a directory", root)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("install: %v", err)
	}

	steps := plan(root, *force)

	fmt.Printf("install %s\n\n", root)
	for _, s := range steps {
		fmt.Printf("  %-9s %s\n", s.act, s.name)
	}
	fmt.Println()

	if !steps.writes() {
		fmt.Println("nothing to do (--force rewrites the generated files)")
		return nil
	}

	if !*yes {
		ok, err := confirm()
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("install: cancelled")
		}
	}

	if err := steps.apply(root); err != nil {
		return err
	}

	fmt.Printf("\ninstalled %s\n", root)
	return nil
}

func confirm() (bool, error) {
	fmt.Print("proceed? [y/N] ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("install: %v", err)
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}
