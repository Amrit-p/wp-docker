package deploy

import "fmt"

type PHP struct{}

var _ Deployer = PHP{}

func (PHP) Deploy(site string) error {
	fmt.Printf("deploying php site %s\n", site)
	return nil
}
