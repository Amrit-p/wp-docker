# wpdock

A command line tool for running WordPress and PHP sites in Docker on a single host.

Each site is one container built from an official image
(`wordpress:<version>-php<php-version>-apache` or `php:<php-version>-apache`). A shared
nginx container reverse-proxies each site's domain to its container, and the sites share
one MariaDB. A site is described entirely by the flags it was added with, which are stored
as labels on its container, so there is no state file to keep in sync.

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

Every command accepts `--prefix`, and it defaults to the current directory — so you can `cd` into
your install tree and leave it off. The commands that read or write files under it (`install`,
`ssl` and `site-add`/`convert`/`update`/`nuke`/`backup`/`restore`/`shell`) use it; the rest (`db`,
`site-list`, `site-details`, `site-stop`) accept it for consistency but act on containers, not
the tree.

## Commands

### install

Creates the directory tree the `site-*` commands write into.

```sh
./main install [--prefix=<path>] [--force] [--yes]
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | no | The directory to install into (default: the current directory). Created if it does not exist. |
| `--force` | no | Rewrite generated files that already exist. |
| `--yes` | no | Skip the confirmation prompt. |

It prints what it is about to do and asks before writing anything. Rerunning it is
safe: without `--force` every file that already exists is skipped.

```
<prefix>/
  docker-compose.yml             the shared nginx + mariadb services, on the wpdock network
  .env                           the values docker-compose.yml and the Makefile read; created once, never rewritten
  .env.example                   the reference copy of .env, kept current by --force
  data/                          per-site docroots: data/<site-name>/, bind-mounted in
  backups/                       site-backup writes the .tgz and .json here
  certs/                         Let's Encrypt state, mounted into certbot as /etc/letsencrypt
  nginx/
    nginx.conf                   generated main config for the shared nginx container
    conf/                        per-site vhosts, written by site-add
    conf.d/
      security.conf              headers and deny rules, included by every vhost
    templates/
      site.conf.tmpl             the vhost every plain-http site renders
      site-ssl.conf.tmpl         the vhost a site renders once `ssl` has its certificate
    logs/                        nginx error, access and pid files
    tmp/                         nginx client body, proxy and fastcgi temp files
  www/                           shared webroot, holds the default page
```

Everything here is regenerated from the binary, so `--force` overwrites local edits to
`nginx.conf`, `security.conf`, the templates or `.env.example`. `data/`, `backups/` and
`certs/` hold your sites, and `.env` holds your settings — those are only ever added to,
never rewritten.

## Shared containers

The `site-*` and `db` commands rely on two long-lived containers on a Docker network named
`wpdock`. `install` writes a `docker-compose.yml` at the prefix that defines both:

| Container | Role |
| --- | --- |
| `wpdock-nginx-1.27` | The reverse proxy on ports 80 and 443. The compose file runs the official `nginx` image with `<prefix>` mounted at the same path (so the absolute paths in `nginx.conf` resolve) via `nginx -c <prefix>/nginx/nginx.conf`. It serves `nginx.conf` and picks up the vhosts `site-add` writes into `conf/`. To apply a new vhost, `site-add` finds the proxy by its `wpdock.role=proxy` label — not its name, so the nginx version can change freely — and sends it `SIGHUP` (`docker kill --signal=HUP`), which the nginx master turns into a graceful reload. |
| `wpdock-mariadb-11` | The shared database. `db --create-user` provisions a database and user in it; each site connects to it with `--db-host=wpdock-mariadb-11`. Its data persists in the `wpdock-mariadb-data` volume. |

Bring them up once, from anywhere:

```sh
docker compose -f <prefix>/docker-compose.yml up -d
```

The compose file reads its settings from the `.env` beside it, which `install` creates from
`.env.example` the first time (docker compose loads a `.env` next to the compose file
automatically, wherever you run it from):

| Variable | Default | What it sets |
| --- | --- | --- |
| `WPDOCK_MARIADB_ROOT_PASSWORD` | `change-me` | MariaDB's root password — the `--root-password` you pass to `db --create-user` and `site-convert`. Change it before the first start. |
| `WPDOCK_MARIADB_PORT` | `3307` | The host port MariaDB is published on, bound to `127.0.0.1` only. It exists for host-side clients (`mysql -h 127.0.0.1 -P 3307`); sites reach MariaDB over the `wpdock` network and never use it. It defaults to 3307 so an old stack's MariaDB can keep 3306 while both run during a migration. |
| `WPDOCK_HTTP_PORT` | `80` | The host port the nginx proxy serves http on. |
| `WPDOCK_HTTPS_PORT` | `443` | The host port the nginx proxy serves https on. `ssl` expects the proxy on 443. |
| `WPDOCK_PHP_VERSION` | `8.3` | Only read by the Makefile: the `--php-version` `make add-site` uses when `PHP_VERSION=` is not given. |
| `WPDOCK_WP_VERSION` | `6.8` | Only read by the Makefile: the `--wp-version` `make add-site` uses when `WP_VERSION=` is not given. |
| `WPDOCK_LETSENCRYPT_EMAIL` | empty | Only read by the Makefile: the `--email` `make ssl` uses when `EMAIL=` is not given. |

Changing a port or the root password takes effect on the next
`docker compose -f <prefix>/docker-compose.yml up -d` (the root password only initializes a
fresh data volume — MariaDB keeps the password it was first started with). `site-add` also runs
`docker network create wpdock` itself if the network is somehow missing.

A site's files are written by its container as `root`/`www-data`, which the `wpdock` binary —
running as you — cannot archive, replace or delete. So `site-backup`, `site-restore` and
`site-nuke` do those file operations inside a throwaway `busybox` container that runs as root,
pulling the small `busybox` image once if it is not already present.

### site-add

Creates and starts a site's container, then routes its domain to it.

```sh
./main site-add --prefix=<path> --name=<site> --domain=<domain> [--aliases=<d,d>] \
  --type=<wordpress|php> [--wp-version=<v>] --php-version=<v> \
  --memory=<m> --cpu=<c> --pids=<n> \
  [--db-host=<h> --db-name=<db> --db-user=<u> --db-password=<p>]
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | no | The directory `install` created (default: the current directory). |
| `--name` | yes | The site name. Becomes the container `wpdock-<name>` and the docroot `data/<name>`. Letters, digits, `-` and `_`. |
| `--domain` | yes | The primary domain nginx routes to the site, e.g. `blog.com`. |
| `--aliases` | no | Comma-separated extra domains, added to `server_name` after `--domain`. |
| `--type` | yes | `wordpress` or `php`, and nothing else. |
| `--wp-version` | wordpress | The WordPress version, e.g. `6.8`. Required for `wordpress`, ignored for `php`. |
| `--php-version` | yes | The PHP version, e.g. `8.3`. |
| `--memory` | no | Memory cap, in Docker's units. Default `512m`. |
| `--cpu` | no | CPU quota. Default `0.5` (half a core). |
| `--pids` | no | Cap on processes in the container. Default `100`. |
| `--db-host` | wordpress | The database host, usually the shared `wpdock-mariadb-11` container. |
| `--db-name` | wordpress | The database the site connects to. It must already exist. |
| `--db-user` | wordpress | The user the site connects as. |
| `--db-password` | wordpress | That user's password. |

The image is `wordpress:<version>-php<php-version>-apache` or `php:<php-version>-apache`.
The container is self-contained: Apache serves both static files and PHP, and nginx just
proxies to it, so the WordPress and PHP vhosts are identical.

`site-add` does **not** create the database. Make it first with `db --create-user` (below),
then pass the same host, name, user and password here. The four `--db-*` values are passed to
the container as `WORDPRESS_DB_HOST/NAME/USER/PASSWORD`; WordPress creates its tables in the
database on first run. For a `php` site the `--db-*` flags are optional, and are forwarded as
the same environment variables if given.

The site's docroot is bind-mounted from `<prefix>/data/<name>`, so its files live on the host
and survive the container being replaced by `site-update` or `site-restore`.

`--name` must be free: if a container `wpdock-<name>` already exists, running or stopped,
`site-add` refuses rather than replace it. Change an existing site with `site-update`, or remove
it with `site-nuke`, first.

```sh
./main db --create-user --db-container=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me --root-password=change-me
./main site-add --prefix=~/wpdock --name=blog --domain=blog.test \
  --type=wordpress --wp-version=6.8 --php-version=8.3 \
  --memory=512m --cpu=1.0 --pids=256 \
  --db-host=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me
```

### ssl

Puts a site on https with a Let's Encrypt certificate.

```sh
./main ssl --prefix=<path> --name=<site> --email=<address> [--staging]
./main ssl --prefix=<path> --renew
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | no | The directory `install` created (default: the current directory). |
| `--name` | to issue | The site to certify. It must already exist. |
| `--email` | to issue | The address Let's Encrypt registers and sends expiry notices to. |
| `--staging` | no | Use the Let's Encrypt staging environment, which issues untrusted test certificates without burning rate limits. |
| `--renew` | to renew | Renew every certificate that is due, instead of issuing one. |

It proves the domain with certbot's webroot challenge, run in a throwaway `certbot/certbot`
container: nginx serves `/.well-known/acme-challenge/` for the site out of `<prefix>/www`,
certbot writes the challenge file there, and Let's Encrypt fetches it over plain http. So the
site's `--domain` and every `--aliases` entry must publicly resolve to this server, and port 80
must be reachable — which rules out local-only domains like `blog.test`. The certificate covers
the domain and all aliases, and lands under `<prefix>/certs`, which certbot sees as its
`/etc/letsencrypt`.

With the certificate on disk, the vhost is rewritten from `site-ssl.conf.tmpl` — http answers
renewal challenges and redirects everything else to https, which proxies to the container the
way the plain vhost did — and nginx reloads. The official WordPress image already trusts the
`X-Forwarded-Proto` header the vhost sends, so WordPress knows it is behind https and needs no
configuration.

The certificate on disk **is** the site's ssl state: nothing is written to the container's
labels. `site-update` and `site-restore` render whichever vhost matches what is in
`<prefix>/certs` at the time, so https survives both; `site-nuke` deletes the site's
certificate along with everything else, so `--renew` never chases a domain that no longer
routes here.

Certificates last 90 days. Renewal is one command, so put it in cron:

```sh
0 3 * * * cd <prefix> && /path/to/main ssl --renew
```

`certbot renew` only touches certificates inside their renewal window, so running it daily is
what certbot itself recommends. The nginx reload after it is what picks a renewed certificate
up.

On an install that predates ssl, the compose file does not publish port 443. `ssl` checks for
that and refuses with the fix spelled out: rerun `install --force` to regenerate
`docker-compose.yml` (and add `site-ssl.conf.tmpl`), then `docker compose -f
<prefix>/docker-compose.yml up -d` to recreate the proxy with the port.

```sh
./main ssl --prefix=~/wpdock --name=blog --email=you@example.com
```

### site-convert

Copies a site off the old make/bash stack (the Makefile and `scripts/` this repo grew out
of) into wpdock: its files, its database and its routing.

```sh
./main site-convert --prefix=<path> --old-prefix=<path> --name=<site> --root-password=<pass> \
  [--db-host=<h>] [--aliases=<d,d>] [--wp-version=<v>] [--php-version=<v>] [--yes]
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | no | The wpdock tree `install` created (default: the current directory). |
| `--old-prefix` | yes | The old stack's root — the directory holding its `sites/` and `nginx/`. |
| `--name` | yes | The site, named as the old stack knows it (container `wp_<name>`, files `sites/<name>/`). |
| `--root-password` | yes | The MariaDB root password of `--db-host`, which creates the database and user there. |
| `--db-host` | no | The wpdock MariaDB container that receives the database. Default `wpdock-mariadb-11`. |
| `--type` | no | Override the `wp.type` label (`wp` or `php`). Old-stack containers from before that label carry none; without the flag the type is inferred from the container's `WORDPRESS_DB_NAME` env, then from whether `sites/<name>/wordpress` or `sites/<name>/app` exists. |
| `--domain` | no | Override the domain read from the old container's `wp.domain` label; required if that label is missing. |
| `--aliases` | no | Override the aliases read back from the old vhost's `# Aliases:` header (comma-separated). |
| `--wp-version` | no | Override the WordPress version, otherwise read from the running old container's `wp-includes/version.php`. |
| `--php-version` | no | Override the PHP version, otherwise read from the running old container's `php -r`. |
| `--memory` | no | Memory cap of the new container. Default `512m` — the old container's limits are not carried over. |
| `--cpu` | no | CPU quota of the new container. Default `0.5`. |
| `--pids` | no | Process cap of the new container. Default `100`. |
| `--yes` | no | Skip the confirmation prompt. |

The old container is the source of truth, the way a wpdock container is: domain and type come
from its `wp.*` labels, the database name, user and password from its environment, and both
versions from executing `php` inside it — which is why it must be running (or both
`--*-version` flags given). Resource limits are the one thing not carried over: the new
container gets wpdock's defaults (or the flags above), not the old caps.

Docker Hub never built an image for every version pair a site can report — security backports
like WordPress 6.1.10 have no image at all, and `php7.4` variants stopped being published in
2023. So convert resolves the detected pair to a tag that exists, pulling it as the proof:
first `wordpress:<wp>-php<php>-apache` exactly, then the `<major.minor>` tag
(`wordpress:6.1-php7.4-apache`), then the bare `wordpress:php<php>-apache` variant. Falling
back is safe because the image's bundled WordPress is never used — the entrypoint only unpacks
it into an *empty* docroot, and the old site's files are copied in first; the image supplies
PHP, Apache and the extensions, nothing more. The site's `wpdock.version` label follows
whichever tag won (for the bare variant, the version bundled in that image), so `site-update`
later regenerates a tag that also exists. Only if the whole ladder misses does convert stop —
before touching the database or files — and ask for explicit `--wp-version`/`--php-version`. It prints the whole plan — old and new
container and image, files, database move — plus warnings, and asks before doing anything.

Then it: dumps the old database (as the site's own user, from whichever MariaDB server the old
site was on, extra `add-db` servers included), creates the same database, user and password on
`--db-host` and imports the dump; copies `sites/<name>/wordpress` (or `app/`) into
`data/<name>`, rechowning from Alpine's `www-data` (82) to Debian's (33) and dropping in the
standard WordPress `.htaccess` if the files have none — the old nginx did the rewriting, so
they won't, and without it Apache 404s every pretty permalink; and starts the wpdock container
and writes the vhost.

The old site is not touched: it keeps running and keeps serving until you cut over. That also
means the copy diverges from the moment it is made — convert close to the cutover, or convert
again. If the wpdock proxy is not running yet (it usually cannot be, the old nginx holds ports
80/443), the vhost sits on disk and is picked up when it starts.

```sh
./main site-convert --prefix=~/wpdock --old-prefix=~/wp-stack --name=blog --root-password=change-me
```

The cutover itself, once every site is converted:

```sh
docker stop wp_nginx                                   # the old proxy releases 80/443
docker compose -f ~/wpdock/docker-compose.yml up -d    # the wpdock proxy takes them
./main ssl --prefix=~/wpdock --name=blog --email=you@example.com   # per site, reissue https
docker stop wp_blog                                    # per site, retire the old container
```

#### what breaks when converting

The two stacks differ by design, so some things change behavior and a few genuinely break:

- **Aliases stop redirecting.** The old stack 301'd every alias to the canonical domain
  (`www.blog.com` → `blog.com`); wpdock puts aliases in `server_name` and serves them.
  WordPress itself still canonicalizes to its `siteurl`, so wp sites mostly keep the redirect
  in practice — a php app does not, unless it does its own.
- **php sites lose their database extensions.** The old stack built `wp-stack-php` with
  `mysqli`, `pdo_mysql`, `gd`, `intl`, `zip` and `opcache`; wpdock runs the stock
  `php:<v>-apache`, which ships none of them. A php site that uses a database will fatal until
  those are installed in the image.
- **php sites' env vars are renamed.** The old stack passed `DB_HOST/DB_NAME/DB_USER/DB_PASSWORD`;
  wpdock passes the same values as `WORDPRESS_DB_*`, whatever the type. An app reading
  `getenv('DB_HOST')` finds nothing until it reads the new names.
- **php sites lose the nginx deny guards.** The old php vhost denied `/vendor/`, `/storage/`,
  dotfiles, `composer.json`, `*.sql`, `*.log` and more; wpdock's shared `security.conf` only
  denies dotfiles. Anything else the app kept next to `index.php` becomes reachable —
  recreate the guards in an `.htaccess`.
- **HTTPS does not carry over.** The old certs live under the old stack's `nginx/ssl` with
  lineages named by *site*; wpdock names lineages by *domain* and manages its own
  `certs/`. Converted sites serve plain HTTP until `ssl --name=<site>` reissues — do it right
  after the cutover, and expect that gap.
- **The hardening profile is thinner.** The old containers ran `--read-only` with tmpfs
  mounts, `--memory-swap` pinned to `--memory`, and `no-new-privileges`; wpdock containers run
  writable with Docker's defaults. Nothing user-visible changes, but a compromised site can
  now write outside its docroot inside its own container.
- **FPM becomes Apache.** nginx stops serving static files directly and `.htaccess` files are
  honored again. That is what makes permalinks work with zero nginx config — and it also means
  any behavior an old site owed to its nginx vhost (custom headers, rewrites added by hand) is
  gone until recreated.
- **Sites on extra MariaDB servers move to MariaDB 11.** The old `make add-db` servers pinned a
  site to an older MariaDB; the dump imports into `--db-host` regardless. Upgrading (10.x → 11)
  is what MariaDB supports; converting a site whose server was *newer* than `--db-host` is not.
- **Only the wpdock feature set carries forward.** `set-resources` (live, no recreate) becomes
  `site-update` (recreates); the old per-site `site-credentials.txt` stays behind — wpdock
  stores the same facts on the container, shown by `site-details`.

### site-update

Recreates a site's container with changed flags, keeping its files.

```sh
./main site-update --prefix=<path> --name=<site> [any site-add flag to change]
```

It reads the current settings back from the container's labels, applies only the flags you
pass, and recreates the container. Because the docroot is a host volume, the site's files and
uploads are untouched, so this is how you move a live site to a new PHP or WordPress version.

```sh
./main site-update --prefix=~/wpdock --name=blog --php-version=8.4
```

### site-list

Lists every wpdock-managed site and its state.

```sh
./main site-list
```

It needs no flags and no `--prefix`: the sites are the containers labelled `wpdock.managed=true`,
so it reads them straight from `docker ps` and lets Docker render the table. Stopped sites are
listed too. It stays to four columns on purpose so it reads well as the list grows; for a
site's versions, resources and database, use `site-details`.

```
NAMES         STATE     type        domain
wpdock-blog   running   wordpress   blog.test
wpdock-shop   exited    php         shop.test
```

### site-details

Shows one site's settings, image and state.

```sh
./main site-details --name=<site>
```

It reads the same labels `site-update` does, so it is the way to see exactly how a site was
run — its versions, resource caps, full database connection and current status — without
`docker inspect`. It prints the `db-password`, so treat its output as you would any secret.

```
site blog

  type        wordpress
  domain      blog.test www.blog.test
  wordpress   6.8
  php         8.3
  resources   memory=512m cpus=1.0 max_pids=256
  db-host     wpdock-mariadb-11
  db-name     blog
  db-user     bloguser
  db-password change-me
  image       wordpress:6.8-php8.3-apache
  status      running
```

A `php` site with no database omits the four `db-*` lines.

### site-shell

Opens a shell **inside** one site's container for a developer, isolated to that one site.

```sh
./main site-shell --prefix=<path> --name=<site> --user=<u> --password=<p> --port=<n>
./main site-shell --prefix=<path> --name=<site> --close
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | no | The directory `install` created (default: the current directory). |
| `--name` | yes | The site to grant access to. It must already exist. |
| `--user` | to open | The login name for the developer. Letters, digits, `-` and `_`. |
| `--password` | to open | The login password. |
| `--port` | to open | The host port to expose SSH on, e.g. `2222`. |
| `--close` | to close | Remove the access instead of granting it. |

The official images have no SSH server, so the first time you open a shell wpdock builds a small
image from the site's base image plus `openssh-server`, `sudo`, `wp-cli` and `composer` (tagged
`wpdock-ssh:<base>`, built once per base and cached). It then recreates the site's container from
that image with `--port` published and sshd running. The container's files are untouched (they
live on the host volume), but the site restarts, so it is briefly unavailable during the swap.

Before any of that, wpdock checks that `--port` is free. If another container already publishes
it, the command fails immediately and leaves the running site untouched — a port clash never
leaves you with a half-replaced container. `site-update` and `site-restore` run the same check
before recreating a shell-enabled site.

The developer logs in over SSH with `--user`/`--password` and lands in the live container as the
web user (`www-data`), so they own every file and `wp-cli` runs without `--allow-root`. They have
passwordless `sudo` for anything else. Because it is the site's own container — with no Docker
socket and no host mounts — they cannot reach any other site's files or shell.

```sh
./main site-shell --prefix=~/wpdock --name=blog --user=dev --password=change-me --port=2222
# the developer then:
ssh -p 2222 dev@your-server        # a shell in blog's container: edit files, wp, composer, sudo
```

The access is stored on the container (`ssh-port`/`ssh-user` labels, password in its env), so it
**survives `site-update`**: a version change rebuilds the ssh image and re-publishes the port.
`site-details` shows it as `shell open (port …, user …)`. `site-shell --close` recreates the
container from the plain base image, dropping sshd and the port.

Three things to weigh before exposing the port:

- **It is password SSH on a public port.** Use a strong password and firewall the port to the
  developer's address (on Lightsail, the instance firewall). 
- **`sudo` gives root *inside that container*.** That is the point of "whole container access",
  but it also means a WordPress compromise reaches container-root. It never reaches the host.
- **`site-backup` of a shell-enabled site commits the ssh image**, so the backup carries sshd.

### site-stop

Stops a site's container, leaving its files and database in place.

```sh
./main site-stop --name=<site>
```

The vhost stays, so nginx returns `502` for the domain until the site runs again. Start it
back up by recreating it with `site-update` (with no changed flags), or with `docker start
wpdock-<name>`.

### site-nuke

Deletes a site's container, its files and its database.

```sh
./main site-nuke --prefix=<path> --name=<site> [--yes]
```

It prints what it is about to delete and asks first, like `install` and `db --truncate`.
`--yes` skips the prompt. It removes the container, deletes `<prefix>/data/<name>`, removes the
vhost and reloads nginx, deletes the site's Let's Encrypt certificate if it has one, and then
drops the database — connecting as the site's own `--db-user`, so it drops only that database
and cannot touch any other on the shared server.

```sh
./main site-nuke --prefix=~/wpdock --name=blog
```

### site-backup

Commits the container to a timestamped image and archives its files.

```sh
./main site-backup --prefix=<path> --name=<site>
```

It writes three things under `<prefix>/backups/`, all sharing one backup ID
(`<name>-<YYYYmmdd-HHMMSS>`):

| Artifact | What it is |
| --- | --- |
| image `wpdock-<name>:<timestamp>` | `docker commit` of the container: its installed packages and PHP config. |
| `<id>.tgz` | The docroot, `data/<name>`, which the image does not capture because it is a mounted volume. |
| `<id>.json` | A manifest of the site's settings, so `site-restore` can rebuild the container. |

The manifest holds the database password, so keep `<prefix>/backups/` as private as any other
copy of a site.

```sh
./main site-backup --prefix=~/wpdock --name=blog
```

### site-restore

Recreates a site from a backup: its image, its files and its routing.

```sh
./main site-restore --prefix=<path> --backupID=<id>
```

It reads the manifest, unpacks the `.tgz` back into `data/<name>`, removes any current
container of that name, starts a new one from the backup image, and rewrites and reloads the
vhost. The database is not part of a backup and is left as it is.

```sh
./main site-restore --prefix=~/wpdock --backupID=blog-20260715-203000
```

### site-extract

Replaces an existing site's files and database from a zip and a sql dump. The site must
already exist — create it with `site-add` first — and `site-extract` overwrites what is there;
it does not build a container or set up routing.

```sh
./main site-extract --prefix=<path> --name=<site> --zip_path=<zip> --sql_path=<sql> [--force=y]
```

| Flag | Required | Description |
| --- | --- | --- |
| `--prefix` | no | The wpdock installation directory (default: the current directory). |
| `--name` | yes | The site to replace, as `site-add` created it. |
| `--zip_path` | yes | The zip whose contents become the docroot. |
| `--sql_path` | with database | The sql dump loaded into the site's database. Required for a site with a database; a `php` site with none takes only the zip and ignores it. |
| `--force` | no | `y` skips the confirmation prompt. |

The paths have no defaults: pass both, or use `make restore-site`, which fills them in from
`~/websites/<name>/<name>.zip` and `.sql`. The zip's contents are the docroot: `wp-config.php`
and `wp-content/` sit at the top level of the zip, not inside a wrapping folder. `site-extract`
reads the site's labels for its database and checks both sources exist before anything is
touched, prints what it will overwrite and asks to confirm, then drops every table in the
database and loads the dump, clears `data/<name>` and unpacks the zip into it as the container's
`www-data` (uid 33), and restarts the container. A `php` site with no database replaces only its
files. The container image, the routing and any certificate are left as they are.

```sh
./main site-extract --prefix=~/wpdock --name=blog \
  --zip_path=~/websites/blog/blog.zip --sql_path=~/websites/blog/blog.sql --force=y
```

### site-wp-list-users

Lists the WordPress users (the `wp_users` table) of a site.

```sh
./main site-wp-list-users --name=<site>
```

It runs `php` inside the running site container, loading WordPress so the query uses the site's
own database connection and table prefix, and prints the ID, login, email, registration date and
display name of each user. The password hash is not shown. The site must be a `wordpress` site;
a `php` site is refused.

```
ID  LOGIN   EMAIL               REGISTERED           NAME
1   admin   admin@example.com   2026-07-16 04:47:41  admin
2   editor  editor@example.com  2026-07-16 04:47:43  editor
```

### site-wp-reset-password

Sets a WordPress user's password.

```sh
./main site-wp-reset-password --name=<site> [--userID=<id> | --user=<login>] --password=<pass>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--name` | yes | The site whose user to change. Must be a `wordpress` site. |
| `--userID` | no | The `ID` from `site-wp-list-users`. |
| `--user` | no | The `LOGIN` from `site-wp-list-users`, if you'd rather name the user than number them. Passing both `--userID` and `--user` is an error. |
| `--password` | yes | The new password. |

With neither `--userID` nor `--user`, the password of the site's first administrator (the
one with the lowest ID) is reset — so
`./main site-wp-reset-password --name=blog --password='new-pass'` recovers a site you are
locked out of, printing the login it picked.

Like `site-wp-list-users`, it runs inside the site container, but calls WordPress's own
`wp_set_password()`, so the password is hashed exactly the way a login or the dashboard would
hash it — not a MySQL `MD5()` shortcut. The user and password are passed to the container as
environment, not baked into the command, so a password with shell metacharacters is safe. It
fails if no user matches.

```sh
./main site-wp-reset-password --name=blog --userID=1 --password='new-pass'
./main site-wp-reset-password --name=blog --user=editor --password='new-pass'
```

### db

Talks to a MariaDB container. It runs one operation per invocation, and the operation is
named by a flag: `--create-user`, `--import` or `--truncate`. Passing two of them is an
error rather than a guess. All three act on the database `--db-name` inside the container
`--db-container`, and none of them read a site's settings, so `db` can be pointed at a
database that no site owns yet.

A site is usually set up, and then reset, like this:

```sh
./main db --create-user --db-container=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me --root-password=change-me
./main db --import      --db-container=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me --sql-file=./blog.sql

./main db --truncate    --db-container=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me --yes
./main db --import      --db-container=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me --sql-file=./blog.sql
```

#### db --create-user

Creates a MariaDB user that can reach one database and nothing else.

```sh
./main db --create-user --db-container=<name> --db-name=<db> --db-user=<user> --db-password=<pass> --root-password=<pass>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--create-user` | yes | The operation to run. |
| `--db-container` | yes | The running MariaDB container to run the statements in, e.g. `wpdock-mariadb-11`. |
| `--db-name` | yes | The database the user is given access to. Created if it does not exist. |
| `--db-user` | yes | The user to create. |
| `--db-password` | yes | The password to give the user. |
| `--root-password` | yes | The password of the container's MariaDB `root` user, which is what creates them. |

`--db-name` and `--db-user` may hold only letters, digits and underscores, and are
capped at MariaDB's own limits of 64 and 80 characters. `--db-password` is free-form;
quotes and backslashes in it are escaped, so it does not need quoting beyond whatever
your shell wants.

The user is granted `ALL PRIVILEGES` on `<db>.*` and nothing wider, so it may do as it
likes inside its own database and cannot read, write or even list any other. It is
created as `<user>@'%'`, because the client is another container rather than localhost.
The database is created `utf8mb4` / `utf8mb4_unicode_ci`.

```sh
./main db --create-user --db-container=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me --root-password=change-me
```

```
db wpdock-mariadb-11

  database  blog
  user      blog@%
  grants    all privileges on blog.* and nothing else
```

Rerunning it is safe. The database and the user are only created if missing, and the
password is reset on every run, so a second run with a different `--db-password` rotates
it and leaves the data alone.

Creating a user is a `root` job, so this operation connects as `root` over the container's
own socket with `--root-password`. That is the `MARIADB_ROOT_PASSWORD` the container was
started with. wpdock never stores it: a root password is the one secret in the stack that
nothing but this operation needs, so it stays off every site's labels and is passed on the
command line, once, when a user is created.

#### db --import

Loads a SQL file into one database.

```sh
./main db --import --db-container=<name> --db-name=<db> --db-user=<user> --db-password=<pass> --sql-file=<path>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--import` | yes | The operation to run. |
| `--db-container` | yes | The running MariaDB container to load the file into. |
| `--db-name` | yes | The database to load it into. It must already exist: `--import` does not create it. |
| `--db-user` | yes | The user to connect as. Usually the one `--create-user` made for `--db-name`. |
| `--db-password` | yes | That user's password. |
| `--sql-file` | yes | The SQL file to load, on the host. It is streamed in, so a large dump is not read into memory. |

There is no `--root-password` here, and that is the point. The import runs as `--db-user`,
which holds privileges on `--db-name` and nowhere else, so MariaDB itself is what stops a
dump from reaching another database. A file carrying `DROP TABLE secrets.crown` fails with
`ERROR 1142 ... DROP command denied` rather than being obeyed. Importing as `root` would
have run it.

`--db-name` becomes the default database, so a dump that is a bare list of `CREATE TABLE`
and `INSERT` statements — what `mysqldump <db>` writes, with no `USE` line — lands in the
right place. A dump that names databases itself (`mysqldump --databases`, or one with `USE`
lines) will only work if it names `--db-name`, since the user may not touch any other.

```sh
./main db --import --db-container=wpdock-mariadb-11 --db-name=blog --db-user=blog --db-password=change-me --sql-file=./blog.sql
```

```
db wpdock-mariadb-11

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
./main db --truncate --db-container=<name> --db-name=<db> --db-user=<user> --db-password=<pass> [--yes]
```

| Flag | Required | Description |
| --- | --- | --- |
| `--truncate` | yes | The operation to run. |
| `--db-container` | yes | The running MariaDB container. |
| `--db-name` | yes | The database to empty. It is emptied, not dropped. |
| `--db-user` | yes | The user to connect as, the same one `--import` uses. |
| `--db-password` | yes | That user's password. |
| `--yes` | no | Skip the confirmation prompt. |

It **drops** every table and view in `--db-name`. It does not run SQL's `TRUNCATE`, which
only deletes rows and leaves the tables standing — that would not help, because the
`CREATE TABLE` statements in a dump would still fail with `Table 'posts' already exists` on
the way back in. Dropping the tables is what makes a plain `mysqldump` reload cleanly.

The database itself survives, and so does the user and its grants, so `--create-user` does
not need running again. Foreign keys are no obstacle: the drop runs with
`FOREIGN_KEY_CHECKS = 0`, so tables that reference each other come out whatever order they
are in.

Like `--import`, it connects as `--db-user` and so can only reach that user's own database.
Pointing it at one the user does not hold is refused by MariaDB with
`ERROR 1044 ... Access denied` rather than obeyed.

It prints what it is about to drop and asks first, in the way `install` does. `--yes` skips
the prompt, for scripts.

```
db wpdock-mariadb-11

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

## Makefile

The Makefile wraps the binary with the old make/bash stack's target names and parameters, so
muscle memory (and any automation calling `make`) keeps working. Each target it carries takes
the old parameters unchanged — new ones were only added, none removed. Parameters are
case-sensitive, exactly as before: `NAME=`, not `name=`. Anything without a target here
(`site-convert`, `site-nuke`, `site-restore`, `site-update`, `site-list`, `ssl --renew`, …)
is run as a direct `./main` command.

Two variables aim it:

| Variable | Default | Meaning |
| --- | --- | --- |
| `PREFIX` | `.` | The install tree (`make install PREFIX=...` creates it). All targets read `$(PREFIX)/.env` for defaults. |
| `WPDOCK` | `./main` | The binary to run, as built by `make build`. |

| Target | Parameters | What it runs |
| --- | --- | --- |
| `build` | | `go build -o main ./src` |
| `install` | `[FORCE=1] [YES=1]` | `install` — writes the tree, `docker-compose.yml`, `.env` and `.env.example` at `PREFIX` |
| `up` / `down` / `restart` / `logs` | | `docker compose -f $(PREFIX)/docker-compose.yml ...` |
| `add-site` | `NAME= DOMAIN= [ALIASES="a b"] [TYPE=wp\|php] [DB=default\|<version>] [PHP_VERSION=] [WP_VERSION=] [MEMORY=] [CPUS=] [PIDS=]` | for `wp`: `db --create-user` (database and user `wp_<name>`, random password) then `site-add`; for `php`: `site-add` alone |
| `set-aliases` | `NAME= [ALIASES="a b"]` | `site-update --aliases=...`; empty `ALIASES` clears them; reminds you to rerun `make ssl` if the site has a certificate |
| `teardown-all` | `[PURGE=1] [WIPE_DB=1] [YES=1]` | `site-nuke` for every managed site; `WIPE_DB=1` also `docker compose down -v` |
| `ssl` | `NAME= [EMAIL=] [STAGING=1]` | `ssl` — `EMAIL` falls back to `WPDOCK_LETSENCRYPT_EMAIL` in `.env` |
| `install-renewal-cron` | `[SCHEDULE='0 3 * * *']` | replaces its own crontab line (tagged `wpdock-renew-ssl`) with one running `ssl --renew` |
| `backup` | `NAME=` | `site-backup` |
| `restore-site` | `NAME= [ZIP=path] [SQL=path] [FORCE=y]` | `site-extract` — `ZIP`/`SQL` default to `~/websites/<name>/<name>.zip` and `.sql`; `FORCE=y` skips the prompt |
| `reset-password` | `NAME= PASSWORD= [USER=]` | `site-wp-reset-password` — with no `USER=`, the first administrator |
| `list-users` | `NAME=` | `site-wp-list-users` |
| `stop` | `NAME=` | `site-stop` |
| `details` | `NAME=` | `site-details` |
| `lint` | | `gofmt -l src` and `go vet ./...` |
| `version` / `bump` | `[PART=major\|minor\|patch]` | prints or increments the `VERSION` file |

What behaves differently from the old stack:

- `ALIASES` is still space-separated on the make command line; the Makefile turns it into the
  comma-separated form the binary's `--aliases` takes. And where the old stack 301-redirected
  aliases to the domain, wpdock serves them as the site.
- `TYPE=wp` maps to `--type=wordpress`; `DB=default` maps to `--db-host=wpdock-mariadb-11` and
  `DB=<version>` to `--db-host=wpdock-mariadb-<version>` (a MariaDB container you run yourself
  on the `wpdock` network).
- `add-site` names the database and its user `wp_<name>` (dashes become underscores) as before,
  creating them with `WPDOCK_MARIADB_ROOT_PASSWORD` from `$(PREFIX)/.env`.
- `reset-password` ignores the `USER` environment variable — only an explicit
  `make reset-password USER=...` names a user, otherwise the first administrator is picked.
- `install-docker`, `remove-site`, `set-resources`, `list-sites`, `add-db`/`remove-db`/`list-dbs`,
  `renew-ssl` and `restore-site` are gone — the site ones are single `./main` commands now
  (`site-nuke`, `site-update`, `site-list`, `ssl --renew`), and Docker/extra MariaDB servers
  are yours to manage.

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
  prompt/
    prompt.go           the shared [y/N] confirmation install, db and site-nuke ask
  commands/
    db/
      db.go             parses the db flags, picks the operation, checks the flags it needs
      mariadb.go        the docker exec that pipes SQL into the container's mariadb client
      user.go           --create-user: the SQL that creates the user and grants it one database
      import.go         --import: streams a sql file in as that user
      truncate.go       --truncate: lists the database's tables, asks, drops them; TruncateAll for site-extract
      drop.go           DropDatabase, which site-nuke calls to drop a site's database
    site/
      site.go           the shared core: flags, docker exec, labels, images, vhosts, backups
      add.go            site-add: runs the container, writes the vhost, reloads nginx
      convert.go        site-convert: copies a site off the old make/bash stack into wpdock
      ssl.go            ssl: obtains a Let's Encrypt certificate and switches the vhost to https
      update.go         site-update: reads the labels back, applies changes, recreates
      list.go           site-list: tabulates every wpdock.managed container
      details.go        site-details: prints one site's labels, image and state
      shell.go          site-shell: opens/closes an in-container ssh shell for a developer
      wp.go             site-wp-list-users / site-wp-reset-password: WordPress user ops via php
      stop.go           site-stop: stops the container
      nuke.go           site-nuke: deletes the container, files, vhost and database
      backup.go         site-backup: commits the image, archives the files, writes a manifest
      restore.go        site-restore: rebuilds a site from a manifest, image and archive
      extract.go        site-extract: replaces a site's files and database from a zip and sql dump
    install/
      install.go        parses the install flags, prints the plan, asks, applies it
      layout.go         the tree install writes, and what --force may rewrite
      assets/           the files it writes, compiled into the binary with go:embed
```

Every command exports its `Run`-style function (`Run`, or `Add`, `Stop`, …) and a matching
`Usage`, which prints its own help text. `main.go` keeps a registry of them, so the top level
help is built from the commands themselves.

A site is described entirely by its flags, which `site-add` stores as `wpdock.*` labels on the
container. `site-update`, `site-nuke`, `site-backup` and `site-restore` read them back with
`docker inspect`, so the container is the single source of truth for how it was run.
