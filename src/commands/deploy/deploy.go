package deploy

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"wpdock/src/prefix"
)

type Deployer interface {
	Deploy(site Site) error
}

var deployers = map[string]Deployer{
	"wordpress": WP{},
	"php":       PHP{},
}

func Usage() {
	fmt.Fprintf(os.Stderr, `  deploy --prefix=<path> --name=<site>
        deploy a site listed in <path>/sites.json. stacks: %s
`, strings.Join(stacks(), ", "))
}

func Run(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fs.Usage = Usage
	dir := fs.String("prefix", "", "directory wpdock was installed into")
	name := fs.String("name", "", "name of the site to deploy")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *dir == "" {
		return fmt.Errorf("deploy: --prefix is required")
	}

	if *name == "" {
		return fmt.Errorf("deploy: --name is required")
	}

	root, err := prefix.Resolve(*dir)
	if err != nil {
		return fmt.Errorf("deploy: %s: %v", *dir, err)
	}

	sites, err := load(root)
	if err != nil {
		return fmt.Errorf("deploy: %v", err)
	}

	site, err := find(sites, *name)
	if err != nil {
		return fmt.Errorf("deploy: %v", err)
	}

	d, ok := deployers[site.Stack]
	if !ok {
		return fmt.Errorf("deploy: %s: unknown stack %q in sites.json (want one of: %s)", site.Name, site.Stack, strings.Join(stacks(), ", "))
	}

	return d.Deploy(site)
}

func stacks() []string {
	names := make([]string, 0, len(deployers))
	for name := range deployers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
