# wpdock

A command line tool for deploying sites.

## Build

```sh
go build -o main ./src
```

This produces a `main` binary in the project root. Run it with `./main`.

## Usage

```sh
./main <command> [flags]
```

Run `./main` with no arguments to list the available commands, or
`./main <command> -h` to see the flags for one command.

## Commands

### install

Creates the directory tree that `deploy` writes into.

```sh
./main install --prefix=<path> [--force] [--yes]
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | yes | The directory to install into. Created if it does not exist. |
| `--force` | no | Rewrite generated files that already exist. Never touches `sites.json`. |
| `--yes` | no | Skip the confirmation prompt. |

It prints what it is about to do and asks before writing anything. Rerunning it is
safe: without `--force` every file that already exists is skipped.

```
<prefix>/
  sites.json                     the deployed sites. created only if missing
  data/                          per-site docroots: data/<site-name>/
  nginx/
    nginx.conf                   generated main config
    conf/                        per-site vhosts, written by deploy
    conf.d/
      fastcgi.conf               fastcgi params, included by every vhost
      security.conf              headers and deny rules, included by every vhost
    templates/
      wp.conf.tmpl               the vhost deploy --stack=wp renders
      php.conf.tmpl              the vhost deploy --stack=php renders
    logs/                        nginx error, access and pid files
    tmp/                         nginx client body, proxy and fastcgi temp files
  www/                           shared webroot, holds the default page
```

`sites.json` is state rather than a generated file, so `--force` leaves it alone;
delete it by hand if you really want a fresh one. Everything else is regenerated
from the binary, so local edits to `nginx.conf`, the snippets or the templates are
lost on `--force`.

### deploy

Deploys a site.

```sh
./main deploy --name=<site> --stack=<stack>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--name` | yes | The name of the site to deploy. |
| `--stack` | yes | Which stack to deploy: `wp` (WordPress) or `php` (plain PHP). |

Examples:

```sh
./main deploy --name=blog --stack=wp   # deploys a WordPress site called "blog"
./main deploy --name=shop --stack=php  # deploys a plain PHP site called "shop"
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The command succeeded. |
| `1` | The command ran but failed, for example a missing `--name`. |
| `2` | The command line was wrong, for example an unknown or missing command. |

## Project layout

```
src/
  main.go               reads the command name and hands the rest of the args to it
  commands/
    deploy/
      deploy.go         parses the deploy flags and picks a stack
      wp.go             the WordPress stack
      php.go            the plain PHP stack
    install/
      install.go        parses the install flags, prints the plan, asks, applies it
      layout.go         the tree install writes, and what --force may rewrite
      assets/           the files it writes, compiled into the binary with go:embed
```

Every command exports two functions: `Run(args []string) error`, which does the
work, and `Usage()`, which prints its own help text. `main.go` keeps a registry of
them, so the top level help is built from the commands themselves.

## Adding a stack

Each stack in the `deploy` package implements the same interface:

```go
type Deployer interface {
	Deploy(site string) error
}
```

To add one, create a file next to `wp.go` and `php.go` with a type that implements
`Deploy`, then register it in the `deployers` map in `deploy.go` under the name you
want to pass to `--stack`.

A stack that serves over nginx also needs a vhost template. Add it to
`src/commands/install/assets/templates/`, list it in the `files` table in
`install/layout.go`, and `install` will write it into `<prefix>/nginx/templates/`.
The template is rendered with:

| Field | Value |
| --- | --- |
| `.Name` | the site name, e.g. `blog` |
| `.ServerName` | the `server_name` value, e.g. `blog.test` |
| `.Root` | the site's docroot, `<prefix>/data/<name>` |
| `.Prefix` | the install prefix |
| `.PHP` | what to hand `.php` to, e.g. `unix:/run/php/php8.3-fpm.sock` |
