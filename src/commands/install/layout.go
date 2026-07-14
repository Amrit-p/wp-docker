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
	{path: "sites.json", asset: "assets/sites.json", keep: true},
	{path: "nginx/nginx.conf", asset: "assets/nginx.conf", render: true},
	{path: "nginx/conf.d/fastcgi.conf", asset: "assets/conf.d/fastcgi.conf"},
	{path: "nginx/conf.d/security.conf", asset: "assets/conf.d/security.conf"},
	{path: "nginx/templates/wp.conf.tmpl", asset: "assets/templates/wp.conf.tmpl"},
	{path: "nginx/templates/php.conf.tmpl", asset: "assets/templates/php.conf.tmpl"},
	{path: "www/index.html", asset: "assets/www/index.html", render: true},
}

func (f file) content(root string) ([]byte, error) {
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
	if err := t.Execute(&buf, struct{ Root string }{root}); err != nil {
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

func (s steps) apply(root string) error {
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

		b, err := st.file.content(root)
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
