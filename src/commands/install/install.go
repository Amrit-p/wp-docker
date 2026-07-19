package install

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"wpdock/src/prefix"
	"wpdock/src/prompt"
)

func Usage() {
	fmt.Fprint(os.Stderr, `  install [--prefix=<path>] [--force] [--yes]
        create the wpdock tree (docker-compose.yml, data/, backups/, nginx/, www/) at <path>
`)
}

func Run(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.Usage = Usage
	dir := fs.String("prefix", ".", "directory to install into (default: current directory)")
	force := fs.Bool("force", false, "rewrite generated files that already exist")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
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
		ok, err := prompt.Confirm("proceed?")
		if err != nil {
			return fmt.Errorf("install: %v", err)
		}
		if !ok {
			return fmt.Errorf("install: cancelled")
		}
	}

	if err := steps.apply(root, prefix.Project(root)); err != nil {
		return err
	}

	fmt.Printf("\ninstalled %s\n", root)
	return nil
}
