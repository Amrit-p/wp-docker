package main

import (
	"fmt"
	"os"
	"sort"

	"wpdock/src/commands/db"
	"wpdock/src/commands/install"
	"wpdock/src/commands/site"
)

var (
	version = "dev"
	commit  = "unknown"
)

type command struct {
	run   func(args []string) error
	usage func()
}

var commands = map[string]command{
	"version":                {run: versionCmd, usage: versionUsage},
	"install":                {run: install.Run, usage: install.Usage},
	"db":                     {run: db.Run, usage: db.Usage},
	"ssl":                    {run: site.SSL, usage: site.SSLUsage},
	"site-add":               {run: site.Add, usage: site.AddUsage},
	"site-convert":           {run: site.Convert, usage: site.ConvertUsage},
	"site-update":            {run: site.Update, usage: site.UpdateUsage},
	"site-list":              {run: site.List, usage: site.ListUsage},
	"site-details":           {run: site.Details, usage: site.DetailsUsage},
	"site-shell":             {run: site.Shell, usage: site.ShellUsage},
	"site-wp-list-users":     {run: site.WPListUsers, usage: site.WPListUsersUsage},
	"site-wp-reset-password": {run: site.WPResetPassword, usage: site.WPResetPasswordUsage},
	"site-stop":              {run: site.Stop, usage: site.StopUsage},
	"site-nuke":              {run: site.Nuke, usage: site.NukeUsage},
	"site-backup":            {run: site.Backup, usage: site.BackupUsage},
	"site-restore":           {run: site.Restore, usage: site.RestoreUsage},
	"site-extract":           {run: site.Extract, usage: site.ExtractUsage},
}

func versionCmd(args []string) error {
	fmt.Printf("wpdock %s %s\n", version, commit)
	return nil
}

func versionUsage() {
	fmt.Fprint(os.Stderr, `  version
        print the version and commit this binary was built from
`)
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
