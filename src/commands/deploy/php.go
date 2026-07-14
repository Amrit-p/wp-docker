package deploy

import "fmt"

type PHP struct{}

var _ Deployer = PHP{}

func (PHP) Deploy(site Site) error {
	fmt.Printf("deploying php site %s\n", site.Name)
	describe(site)
	return nil
}
