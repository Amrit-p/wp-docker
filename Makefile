.PHONY: build install up down restart logs add-site set-aliases teardown-all ssl install-renewal-cron backup reset-password list-users stop details lint version bump

PREFIX ?= .
WPDOCK ?= ./main
COMPOSE = docker compose -f $(PREFIX)/docker-compose.yml

-include $(PREFIX)/.env
WPDOCK_MARIADB_ROOT_PASSWORD ?= change-me
WPDOCK_PHP_VERSION ?= 8.3
WPDOCK_WP_VERSION ?= 6.8

TYPE ?= wp
DB ?= default
USER :=

empty :=
space := $(empty) $(empty)
comma := ,
ALIASES_CSV = $(subst $(space),$(comma),$(strip $(ALIASES)))
SITE_TYPE = $(if $(filter wp,$(TYPE)),wordpress,$(TYPE))
DB_HOST = $(if $(filter default,$(DB)),wpdock-mariadb-11,wpdock-mariadb-$(DB))
DB_NAME = wp_$(subst -,_,$(NAME))
PHP_VER = $(if $(PHP_VERSION),$(PHP_VERSION),$(WPDOCK_PHP_VERSION))
WP_VER = $(if $(WP_VERSION),$(WP_VERSION),$(WPDOCK_WP_VERSION))

version:
	@cat VERSION

bump:
	@[ -n "$(PART)" ] || (echo "PART is required: make bump PART=major|minor|patch" && exit 1)
	@current=$$(cat VERSION); \
	major=$${current%%.*}; rest=$${current#*.}; minor=$${rest%%.*}; patch=$${rest#*.}; \
	case "$(PART)" in \
		major) new="$$((major + 1)).0.0" ;; \
		minor) new="$$major.$$((minor + 1)).0" ;; \
		patch) new="$$major.$$minor.$$((patch + 1))" ;; \
		*) echo "Invalid PART '$(PART)': expected major, minor, or patch" && exit 1 ;; \
	esac; \
	echo "$$new" > VERSION; \
	echo "$$current -> $$new"

build:
	go build -o main ./src

install:
	$(WPDOCK) install --prefix="$(PREFIX)" $(if $(FORCE),--force) $(if $(YES),--yes)

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

restart:
	$(COMPOSE) restart

logs:
	$(COMPOSE) logs -f

add-site:
	@[ -z "$(name)$(domain)$(aliases)$(type)$(db)$(php_version)$(wp_version)$(memory)$(cpus)$(pids)" ] || \
		(echo "make parameters are case-sensitive - use NAME= DOMAIN= ALIASES= TYPE= DB= PHP_VERSION= WP_VERSION= MEMORY= CPUS= PIDS=" && exit 1)
	@[ -n "$(NAME)" ] || (echo "NAME is required: make add-site NAME=site1 DOMAIN=site1.example.com" && exit 1)
	@[ -n "$(DOMAIN)" ] || (echo "DOMAIN is required: make add-site NAME=site1 DOMAIN=site1.example.com" && exit 1)
	@set -e; \
	if [ "$(SITE_TYPE)" = "wordpress" ]; then \
		pass=$$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24); \
		$(WPDOCK) db --create-user --db-container="$(DB_HOST)" --db-name="$(DB_NAME)" --db-user="$(DB_NAME)" --db-password="$$pass" --root-password="$(WPDOCK_MARIADB_ROOT_PASSWORD)"; \
		$(WPDOCK) site-add --prefix="$(PREFIX)" --name="$(NAME)" --domain="$(DOMAIN)" --type=wordpress \
			$(if $(ALIASES),--aliases="$(ALIASES_CSV)") \
			--wp-version="$(WP_VER)" --php-version="$(PHP_VER)" \
			$(if $(MEMORY),--memory="$(MEMORY)") $(if $(CPUS),--cpu="$(CPUS)") $(if $(PIDS),--pids="$(PIDS)") \
			--db-host="$(DB_HOST)" --db-name="$(DB_NAME)" --db-user="$(DB_NAME)" --db-password="$$pass"; \
	else \
		$(WPDOCK) site-add --prefix="$(PREFIX)" --name="$(NAME)" --domain="$(DOMAIN)" --type="$(SITE_TYPE)" \
			$(if $(ALIASES),--aliases="$(ALIASES_CSV)") \
			--php-version="$(PHP_VER)" \
			$(if $(MEMORY),--memory="$(MEMORY)") $(if $(CPUS),--cpu="$(CPUS)") $(if $(PIDS),--pids="$(PIDS)"); \
	fi

set-aliases:
	@[ -n "$(NAME)" ] || (echo "NAME is required: make set-aliases NAME=site1 ALIASES=\"www.site1.example.com\"" && exit 1)
	$(WPDOCK) site-update --prefix="$(PREFIX)" --name="$(NAME)" --aliases="$(ALIASES_CSV)"
	@domain=$$(docker inspect -f '{{index .Config.Labels "wpdock.domain"}}' "wpdock-$(NAME)"); \
	[ ! -f "$(PREFIX)/certs/renewal/$$domain.conf" ] || \
		echo "https is on for $$domain - rerun 'make ssl NAME=$(NAME)' so the certificate covers the new aliases"

teardown-all:
	@if [ -z "$(YES)" ]; then \
		printf "tear down every wpdock site$(if $(WIPE_DB), and wipe the shared services and database data,)? [y/N] "; \
		read -r a; case "$$a" in y|Y) ;; *) echo cancelled; exit 1;; esac; \
	fi
	@set -e; \
	for s in $$(docker ps -a --filter "label=wpdock.managed=true" --format '{{.Names}}' | sed 's/^wpdock-//'); do \
		$(WPDOCK) site-nuke --prefix="$(PREFIX)" --name="$$s" --yes; \
	done
	@if [ -n "$(WIPE_DB)" ]; then $(COMPOSE) down -v; fi

ssl:
	@[ -n "$(NAME)" ] || (echo "NAME is required: make ssl NAME=site1" && exit 1)
	@email="$(if $(EMAIL),$(EMAIL),$(WPDOCK_LETSENCRYPT_EMAIL))"; \
	[ -n "$$email" ] || { echo "EMAIL is required: make ssl NAME=site1 EMAIL=you@example.com (or set WPDOCK_LETSENCRYPT_EMAIL in $(PREFIX)/.env)"; exit 1; }; \
	$(WPDOCK) ssl --prefix="$(PREFIX)" --name="$(NAME)" --email="$$email" $(if $(STAGING),--staging)

install-renewal-cron:
	@set -e; \
	schedule='$(if $(SCHEDULE),$(SCHEDULE),0 3 * * *)'; \
	prefix=$$(cd "$(PREFIX)" && pwd); \
	bin=$$(cd "$$(dirname "$(WPDOCK)")" && pwd)/$$(basename "$(WPDOCK)"); \
	( crontab -l 2>/dev/null | grep -v 'wpdock-renew-ssl'; echo "$$schedule $$bin ssl --renew --prefix=$$prefix # wpdock-renew-ssl" ) | crontab -; \
	echo "installed: $$schedule $$bin ssl --renew --prefix=$$prefix"

backup:
	@[ -n "$(NAME)" ] || (echo "NAME is required: make backup NAME=site1" && exit 1)
	$(WPDOCK) site-backup --prefix="$(PREFIX)" --name="$(NAME)"

reset-password:
	@[ -n "$(NAME)" ] || (echo "NAME is required: make reset-password NAME=site1 PASSWORD=newpass [USER=admin]" && exit 1)
	@[ -n "$(PASSWORD)" ] || (echo "PASSWORD is required: make reset-password NAME=site1 PASSWORD=newpass [USER=admin]" && exit 1)
	$(WPDOCK) site-wp-reset-password --name="$(NAME)" $(if $(USER),--user="$(USER)") --password="$(PASSWORD)"

list-users:
	@[ -n "$(NAME)" ] || (echo "NAME is required: make list-users NAME=site1" && exit 1)
	$(WPDOCK) site-wp-list-users --name="$(NAME)"

stop:
	@[ -n "$(NAME)" ] || (echo "NAME is required: make stop NAME=site1" && exit 1)
	$(WPDOCK) site-stop --name="$(NAME)"

details:
	@[ -n "$(NAME)" ] || (echo "NAME is required: make details NAME=site1" && exit 1)
	$(WPDOCK) site-details --name="$(NAME)"

lint:
	@test -z "$$(gofmt -l src)" || (gofmt -l src && exit 1)
	go vet ./...
