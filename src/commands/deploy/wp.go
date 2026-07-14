package deploy

import "fmt"

type WP struct{}

var _ Deployer = WP{}

func (WP) Deploy(site Site) error {
	if site.WordPress == "" {
		return fmt.Errorf("%s: wordpress sites need a \"wordpress\" version in sites.json", site.Name)
	}

	fmt.Printf("deploying wordpress site %s\n", site.Name)
	fmt.Printf("  wordpress  %s\n", site.WordPress)
	describe(site)
	return nil
}
