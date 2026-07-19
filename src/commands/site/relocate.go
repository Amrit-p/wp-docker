package site

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"wpdock/src/commands/install"
	"wpdock/src/prefix"
	"wpdock/src/prompt"
)

func RelocateUsage() {
	fmt.Fprint(os.Stderr, `  relocate --prefix=<path> [--to=<path>] [--yes]
        move an install to a new directory (or repair one in place) and re-point the
        compose stack, every site's container and vhost at it
`)
}

func Relocate(args []string) error { return fail("relocate", relocate(args)) }

func relocate(args []string) error {
	fs := flag.NewFlagSet("relocate", flag.ContinueOnError)
	dir := fs.String("prefix", ".", "current install directory")
	to := fs.String("to", "", "new directory to move the install into (omit to repair it in place)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	fs.Usage = RelocateUsage
	if ok, err := parse(fs, args); !ok {
		return err
	}

	from, err := prefix.Resolve(*dir)
	if err != nil {
		return err
	}
	if !exists(filepath.Join(from, "docker-compose.yml")) {
		return fmt.Errorf("%s is not a wpdock install (no docker-compose.yml)", from)
	}

	root := from
	move := false
	if *to != "" {
		root, err = prefix.Resolve(*to)
		if err != nil {
			return err
		}
		move = root != from
	}
	if move && exists(root) {
		return fmt.Errorf("%s already exists — pick a --to that does not exist yet", root)
	}

	project := composeProject(from)
	sites, err := managedSites()
	if err != nil {
		return err
	}

	fmt.Printf("relocate %s\n\n", from)
	if move {
		field("move", fmt.Sprintf("%s -> %s", from, root))
	} else {
		field("repair", root)
	}
	field("project", fmt.Sprintf("%s (mariadb data stays in the %s_wpdock-mariadb-data volume)", project, project))
	if len(sites) == 0 {
		field("sites", "none")
	} else {
		field("sites", fmt.Sprintf("recreate %d and rewrite their vhosts: %s", len(sites), strings.Join(sites, ", ")))
	}
	fmt.Println()

	if !*yes {
		ok, err := prompt.Confirm("relocate it?")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cancelled")
		}
	}

	if move {
		if err := os.Rename(from, root); err != nil {
			return fmt.Errorf("moving %s to %s: %v (move it by hand, then `wpdock relocate --prefix=%s` to repair in place)", from, root, err, root)
		}
	}

	if err := install.Rerender(root, project); err != nil {
		return err
	}

	for _, name := range sites {
		c, err := inspect(name)
		if err != nil {
			return err
		}
		var img string
		if err := inspectJSON(name, "{{json .Config.Image}}", &img); err != nil {
			return err
		}
		if err := docker("rm", "-f", container(name)); err != nil {
			return err
		}
		if err := run(root, c, img); err != nil {
			return err
		}
		if err := writeVhost(root, c); err != nil {
			return err
		}
		fmt.Printf("  re-pointed %s\n", name)
	}

	fmt.Println("\nrecreating the compose stack ...")
	if err := composeUp(root); err != nil {
		return err
	}

	fmt.Printf("\nrelocated to %s\n", root)
	if move {
		fmt.Printf("\nrepoint anything still passing --prefix=%s (a cron `ssl --renew`, your convert-all.sh) at --prefix=%s\n", from, root)
	}
	return nil
}

func composeProject(from string) string {
	candidates := []string{"wpdock-mariadb-11"}
	if n := proxyName(); n != "" {
		candidates = append([]string{n}, candidates...)
	}
	for _, name := range candidates {
		var p string
		if inspectContainer(name, `{{json (index .Config.Labels "com.docker.compose.project")}}`, &p) == nil && p != "" {
			return p
		}
	}
	return prefix.Project(from)
}

func proxyName() string {
	out, err := dockerOut("ps", "-a", "--filter", "label="+proxyLabel, "--format", "{{.Names}}")
	if err != nil {
		return ""
	}
	if fields := strings.Fields(out); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func managedSites() ([]string, error) {
	out, err := dockerOut("ps", "-a", "--filter", "label="+labelPrefix+"managed=true", "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, n := range strings.Fields(out) {
		names = append(names, strings.TrimPrefix(n, namePrefix))
	}
	return names, nil
}

func composeUp(root string) error {
	cmd := exec.Command("docker", "compose", "-f", filepath.Join(root, "docker-compose.yml"), "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %v", err)
	}
	return nil
}
