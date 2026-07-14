package main

import (
	"fmt"
	"os"
	"sort"

	"wpdock/src/commands/db"
	"wpdock/src/commands/deploy"
	"wpdock/src/commands/install"
)

type command struct {
	run   func(args []string) error
	usage func()
}

var commands = map[string]command{
	"db":      {run: db.Run, usage: db.Usage},
	"deploy":  {run: deploy.Run, usage: deploy.Usage},
	"install": {run: install.Run, usage: install.Usage},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	name, args := os.Args[1], os.Args[2:]

	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", name)
		usage()
		os.Exit(2)
	}

	if err := cmd.run(args); err != nil {
		fmt.Fprintf(os.Stderr, "wpdock: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, "usage: wpdock <command> [flags]\n\ncommands:\n")

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		commands[name].usage()
	}
}
