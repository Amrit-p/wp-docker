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

### db

Talks to a MariaDB container. It runs one operation per invocation, and the operation is
named by a flag: `--create-user`, `--import` or `--truncate`. Passing two of them is an
error rather than a guess. All three act on the database `--db_name` inside the container
`--db_container`, and none of them read `sites.json`, so `db` can be pointed at a database
that no site owns yet.

A site is usually set up, and then reset, like this:

```sh
./main db --create-user --db_container=wpdock-mariadb --db_name=blog --db_user=blog --db_password=change-me --root_password=change-me
./main db --import      --db_container=wpdock-mariadb --db_name=blog --db_user=blog --db_password=change-me --sql_file=./blog.sql

./main db --truncate    --db_container=wpdock-mariadb --db_name=blog --db_user=blog --db_password=change-me --yes
./main db --import      --db_container=wpdock-mariadb --db_name=blog --db_user=blog --db_password=change-me --sql_file=./blog.sql
```

#### db --create-user

Creates a MariaDB user that can reach one database and nothing else.

```sh
./main db --create-user --db_container=<name> --db_name=<db> --db_user=<user> --db_password=<pass> --root_password=<pass>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--create-user` | yes | The operation to run. |
| `--db_container` | yes | The running MariaDB container to run the statements in, e.g. `wpdock-mariadb`. |
| `--db_name` | yes | The database the user is given access to. Created if it does not exist. |
| `--db_user` | yes | The user to create. |
| `--db_password` | yes | The password to give the user. |
| `--root_password` | yes | The password of the container's MariaDB `root` user, which is what creates them. |

`--db_name` and `--db_user` may hold only letters, digits and underscores, and are
capped at MariaDB's own limits of 64 and 80 characters. `--db_password` is free-form;
quotes and backslashes in it are escaped, so it does not need quoting beyond whatever
your shell wants.

The user is granted `ALL PRIVILEGES` on `<db>.*` and nothing wider, so it may do as it
likes inside its own database and cannot read, write or even list any other. It is
created as `<user>@'%'`, because the client is another container rather than localhost.
The database is created `utf8mb4` / `utf8mb4_unicode_ci`.

```sh
./main db --create-user --db_container=wpdock-mariadb --db_name=blog --db_user=blog --db_password=change-me --root_password=change-me
```

```
db wpdock-mariadb

  database  blog
  user      blog@%
  grants    all privileges on blog.* and nothing else
```

Rerunning it is safe. The database and the user are only created if missing, and the
password is reset on every run, so a second run with a different `--db_password` rotates
it and leaves the data alone.

Creating a user is a `root` job, so this operation connects as `root` over the container's
own socket with `--root_password`. That is the `MARIADB_ROOT_PASSWORD` the container was
started with, which for a site listed in `sites.json` is the one in its `wpdock-mariadb`
container's `env`.

#### db --import

Loads a SQL file into one database.

```sh
./main db --import --db_container=<name> --db_name=<db> --db_user=<user> --db_password=<pass> --sql_file=<path>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--import` | yes | The operation to run. |
| `--db_container` | yes | The running MariaDB container to load the file into. |
| `--db_name` | yes | The database to load it into. It must already exist: `--import` does not create it. |
| `--db_user` | yes | The user to connect as. Usually the one `--create-user` made for `--db_name`. |
| `--db_password` | yes | That user's password. |
| `--sql_file` | yes | The SQL file to load, on the host. It is streamed in, so a large dump is not read into memory. |

There is no `--root_password` here, and that is the point. The import runs as `--db_user`,
which holds privileges on `--db_name` and nowhere else, so MariaDB itself is what stops a
dump from reaching another database. A file carrying `DROP TABLE secrets.crown` fails with
`ERROR 1142 ... DROP command denied` rather than being obeyed. Importing as `root` would
have run it.

`--db_name` becomes the default database, so a dump that is a bare list of `CREATE TABLE`
and `INSERT` statements — what `mysqldump <db>` writes, with no `USE` line — lands in the
right place. A dump that names databases itself (`mysqldump --databases`, or one with `USE`
lines) will only work if it names `--db_name`, since the user may not touch any other.

```sh
./main db --import --db_container=wpdock-mariadb --db_name=blog --db_user=blog --db_password=change-me --sql_file=./blog.sql
```

```
db wpdock-mariadb

  database  blog
  user      blog@%
  imported  ./blog.sql (2.4 MiB)
```

Two things this does not do, both inherited from the MariaDB client rather than chosen:

An import is **not** a transaction. The file is fed to the client statement by statement,
so if one fails, the statements before it have already been applied and stay applied. The
command stops there and exits non-zero, printing the statement MariaDB objected to and
why. Fix the file and reimport into a clean database rather than assuming a failed import
left nothing behind.

An import is **not** idempotent. Rerunning a dump full of `CREATE TABLE` fails the second
time with `Table 'posts' already exists`, because that is what the dump asked for. Dumps
written with `DROP TABLE IF EXISTS` (`mysqldump --add-drop-table`, which is the default)
reload cleanly. `--truncate` is the way to reload one that does not.

#### db --truncate

Empties a database so that it can be imported into again.

```sh
./main db --truncate --db_container=<name> --db_name=<db> --db_user=<user> --db_password=<pass> [--yes]
```

| Flag | Required | Description |
| --- | --- | --- |
| `--truncate` | yes | The operation to run. |
| `--db_container` | yes | The running MariaDB container. |
| `--db_name` | yes | The database to empty. It is emptied, not dropped. |
| `--db_user` | yes | The user to connect as, the same one `--import` uses. |
| `--db_password` | yes | That user's password. |
| `--yes` | no | Skip the confirmation prompt. |

It **drops** every table and view in `--db_name`. It does not run SQL's `TRUNCATE`, which
only deletes rows and leaves the tables standing — that would not help, because the
`CREATE TABLE` statements in a dump would still fail with `Table 'posts' already exists` on
the way back in. Dropping the tables is what makes a plain `mysqldump` reload cleanly.

The database itself survives, and so does the user and its grants, so `--create-user` does
not need running again. Foreign keys are no obstacle: the drop runs with
`FOREIGN_KEY_CHECKS = 0`, so tables that reference each other come out whatever order they
are in.

Like `--import`, it connects as `--db_user` and so can only reach that user's own database.
Pointing it at one the user does not hold is refused by MariaDB with
`ERROR 1044 ... Access denied` rather than obeyed.

It prints what it is about to drop and asks first, in the way `install` does. `--yes` skips
the prompt, for scripts.

```
db wpdock-mariadb

  database  blog
  user      blog@%
  drop      2 tables, 1 view

    recent
    comments
    posts

drop them? [y/N]
```

Answering anything but `y` or `yes` cancels and drops nothing. On an already empty database
it reports `nothing to drop` and exits 0, so it is safe to put in front of an import
unconditionally.

Stored routines and events are not dropped, only tables and views. A `mysqldump` of a
WordPress database contains neither, but a dump that defines a procedure will still clash
on reload.

#### passwords on the command line

Passwords are handed to the client in the `MYSQL_PWD` environment variable rather than as a
`-p` argument, so they do not appear in the container's process list, and a password holding
quotes or spaces needs no special care. They are still on `wpdock`'s own command line,
though, which is the price of a flag: they will show in your shell history and, while the
command runs, in `ps` on the host. Prefer a shell configured not to record them, or read
them from a variable rather than typing them literally.

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
    db/
      db.go             parses the db flags, picks the operation, checks the flags it needs
      mariadb.go        the docker exec that pipes SQL into the container's mariadb client
      user.go           --create-user: the SQL that creates the user and grants it one database
      import.go         --import: streams a sql file in as that user
      truncate.go       --truncate: lists the database's tables, asks, drops them
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
