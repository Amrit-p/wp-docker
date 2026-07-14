package deploy

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type Deployer interface {
	Deploy(site string) error
}

var deployers = map[string]Deployer{
	"wp":  WP{},
	"php": PHP{},
}

func Usage() {
	fmt.Fprintf(os.Stderr, `  deploy --name=<site> --stack=<stack>
        deploy a site. stacks: %s
`, strings.Join(stacks(), ", "))
}

func Run(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fs.Usage = Usage
	site := fs.String("name", "", "name of the site to deploy")
	stack := fs.String("stack", "", "stack to deploy: "+strings.Join(stacks(), ", "))

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *site == "" {
		return fmt.Errorf("deploy: --name is required")
	}

	if *stack == "" {
		return fmt.Errorf("deploy: --stack is required (want one of: %s)", strings.Join(stacks(), ", "))
	}

	d, ok := deployers[*stack]
	if !ok {
		return fmt.Errorf("deploy: unknown stack %q (want one of: %s)", *stack, strings.Join(stacks(), ", "))
	}

	return d.Deploy(*site)
}

func stacks() []string {
	names := make([]string, 0, len(deployers))
	for name := range deployers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
