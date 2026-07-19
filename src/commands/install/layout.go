package install

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed assets
var assets embed.FS

var dirs = []string{
	"data",
	"backups",
	"certs",
	"nginx",
	"nginx/conf",
	"nginx/conf.d",
	"nginx/logs",
	"nginx/templates",
	"nginx/tmp",
	"www",
}

type file struct {
	path   string
	asset  string
	render bool
	keep   bool
}

var files = []file{
	{path: "Makefile", asset: "assets/Makefile"},
	{path: "docker-compose.yml", asset: "assets/docker-compose.yml", render: true},
	{path: ".env.example", asset: "assets/env.example"},
	{path: ".env", asset: "assets/env.example", keep: true},
	{path: "nginx/nginx.conf", asset: "assets/nginx.conf", render: true},
	{path: "nginx/conf.d/security.conf", asset: "assets/conf.d/security.conf"},
	{path: "nginx/templates/site.conf.tmpl", asset: "assets/templates/site.conf.tmpl"},
	{path: "nginx/templates/site-ssl.conf.tmpl", asset: "assets/templates/site-ssl.conf.tmpl"},
	{path: "www/index.html", asset: "assets/www/index.html", render: true},
}

func (f file) content(root, project string) ([]byte, error) {
	b, err := assets.ReadFile(f.asset)
	if err != nil {
		return nil, err
	}
	if !f.render {
		return b, nil
	}

	t, err := template.New(f.path).Parse(string(b))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, struct{ Root, Project string }{root, project}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type action int

const (
	create action = iota
	rewrite
	skip
)

func (a action) String() string {
	switch a {
	case create:
		return "create"
	case rewrite:
		return "rewrite"
	}
	return "skip"
}

type step struct {
	act  action
	name string
	file *file
}

type steps []step

func plan(root string, force bool) steps {
	out := make(steps, 0, len(dirs)+len(files))

	for _, dir := range dirs {
		act := create
		if exists(filepath.Join(root, dir)) {
			act = skip
		}
		out = append(out, step{act: act, name: dir + "/"})
	}

	for _, f := range files {
		var act action
		switch {
		case !exists(filepath.Join(root, f.path)):
			act = create
		case f.keep, !force:
			act = skip
		default:
			act = rewrite
		}
		out = append(out, step{act: act, name: f.path, file: &f})
	}

	return out
}

func (s steps) writes() bool {
	for _, st := range s {
		if st.act != skip {
			return true
		}
	}
	return false
}

func (s steps) apply(root, project string) error {
	for _, st := range s {
		if st.act == skip {
			continue
		}

		path := filepath.Join(root, st.name)

		if st.file == nil {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return fmt.Errorf("install: %v", err)
			}
			continue
		}

		b, err := st.file.content(root, project)
		if err != nil {
			return fmt.Errorf("install: %s: %v", st.name, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("install: %v", err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return fmt.Errorf("install: %v", err)
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func Rerender(root, project string) error {
	for _, f := range files {
		if !f.render {
			continue
		}
		b, err := f.content(root, project)
		if err != nil {
			return fmt.Errorf("install: %s: %v", f.path, err)
		}
		path := filepath.Join(root, f.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("install: %v", err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return fmt.Errorf("install: %v", err)
		}
	}
	return nil
}
