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
  sites.json                     the sites and how to run them. created only if missing
  data/                          per-site docroots: data/<site-name>/
  nginx/
    nginx.conf                   generated main config
    conf/                        per-site vhosts, written by deploy
    conf.d/
      fastcgi.conf               fastcgi params, included by every vhost
      security.conf              headers and deny rules, included by every vhost
    templates/
      wordpress.conf.tmpl        the vhost the wordpress stack renders
      php.conf.tmpl              the vhost the php stack renders
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
./main deploy --prefix=<path> --name=<site>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | yes | The directory `install` created. `deploy` reads `<prefix>/sites.json`. |
| `--name` | yes | The name of the site to deploy. Must be listed in `sites.json`. |

There is no `--stack` flag. `deploy` looks `--name` up in `<prefix>/sites.json` and
uses the entry it finds, so a site is described in one place rather than on every
command line. Deploying a name that is not listed is an error, as is a listed site
whose `stack` is not one the binary knows.

`install` writes a `sites.json` holding one `example` site. It is there to be copied
and renamed, and it is a real entry: `deploy --name=example` will deploy it.

`sites` is an object, not a list: each key is a site name, and `--name` is looked up
against those keys.

```json
{
  "sites": {
    "example": {
      "stack": "wordpress",
      "wordpress": "6.8",
      "php": "8.3",
      "containers": [
        {
          "name": "wpdock-nginx",
          "env": {
            "NGINX_HOST": "example.test"
          }
        },
        {
          "name": "wpdock-mariadb",
          "env": {
            "MARIADB_DATABASE": "example",
            "MARIADB_USER": "example",
            "MARIADB_PASSWORD": "change-me",
            "MARIADB_ROOT_PASSWORD": "change-me"
          }
        }
      ],
      "resources": {
        "memory": "512m",
        "cpus": "1.0",
        "max_pids": 256
      }
    }
  }
}
```

| Field | Description |
| --- | --- |
| `stack` | Which stack deploys it: `wordpress` or `php` (plain PHP). |
| `wordpress` | The WordPress version to run, e.g. `6.8`. Required by the `wordpress` stack, ignored by `php`. |
| `php` | The PHP version to run, e.g. `8.3`. |
| `containers` | The containers the site runs on, below. |
| `resources` | The limits every one of them gets, below. |

| `containers` field | Description |
| --- | --- |
| `name` | The container name, e.g. `wpdock-nginx`. |
| `env` | The environment it is started with, as a `KEY: value` object. May be omitted. |

| `resources` field | Description |
| --- | --- |
| `memory` | Memory cap, in Docker's units, e.g. `512m`. |
| `cpus` | CPU quota, e.g. `1.0` for one core. |
| `max_pids` | Cap on processes in the container, e.g. `256`. |

Nothing labels a container by role: `containers` is an ordered list of names and
environments, so a stack that needs to tell nginx from MariaDB does so by name. The
example site's `MARIADB_PASSWORD` is `change-me`, which is exactly what it means.

```sh
./main deploy --prefix=~/wpdock --name=example
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
  prefix/
    prefix.go           turns a --prefix into an absolute path, expanding a leading ~
  commands/
    deploy/
      deploy.go         parses the deploy flags and picks the stack the site asked for
      sites.go          reads sites.json and looks a site up by name
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
want to write as a site's `stack` in `sites.json`.

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
