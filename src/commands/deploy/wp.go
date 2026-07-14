package deploy

import "fmt"

type WP struct{}

var _ Deployer = WP{}

func (WP) Deploy(site string) error {
	fmt.Printf("deploying wordpress site %s\n", site)
	return nil
}
