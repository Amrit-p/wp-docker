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
