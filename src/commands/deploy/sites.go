package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Resources struct {
	Memory  string `json:"memory"`
	CPUs    string `json:"cpus"`
	MaxPIDs int    `json:"max_pids"`
}

func (r Resources) String() string {
	return fmt.Sprintf("memory=%s cpus=%s max_pids=%d", r.Memory, r.CPUs, r.MaxPIDs)
}

type Container struct {
	Name string            `json:"name"`
	Env  map[string]string `json:"env"`
}

type Site struct {
	Name       string      `json:"-"`
	Stack      string      `json:"stack"`
	WordPress  string      `json:"wordpress"`
	PHP        string      `json:"php"`
	Containers []Container `json:"containers"`
	Resources  Resources   `json:"resources"`
}

type catalog struct {
	Sites map[string]Site `json:"sites"`
}

func load(root string) (map[string]Site, error) {
	path := filepath.Join(root, "sites.json")

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}

	return c.Sites, nil
}

func find(sites map[string]Site, name string) (Site, error) {
	site, ok := sites[name]
	if ok {
		site.Name = name
		return site, nil
	}

	if len(sites) == 0 {
		return Site{}, fmt.Errorf("unknown site %q (sites.json lists no sites)", name)
	}

	names := make([]string, 0, len(sites))
	for n := range sites {
		names = append(names, n)
	}
	sort.Strings(names)

	return Site{}, fmt.Errorf("unknown site %q (sites.json lists: %s)", name, strings.Join(names, ", "))
}

func describe(site Site) {
	fmt.Printf("  php        %s\n", site.PHP)
	fmt.Printf("  resources  %s\n", site.Resources)

	if len(site.Containers) == 0 {
		fmt.Println("  containers none")
		return
	}

	fmt.Println("  containers")
	for _, c := range site.Containers {
		fmt.Printf("    %s\n", c.Name)

		keys := make([]string, 0, len(c.Env))
		for k := range c.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Printf("      %s=%s\n", k, c.Env[k])
		}
	}
}
