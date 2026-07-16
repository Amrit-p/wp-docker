#!/bin/sh
cd "$(dirname "$0")/.." || exit 1

EMAIL="$1"
[ -n "$EMAIL" ] || { echo "usage: bash migrate/ssl-all.sh you@example.com"; exit 1; }

fail=0
for c in $(docker ps --filter label=wpdock.managed=true --format '{{.Names}}' | sort); do
  name=${c#wpdock-}
  d=$(docker inspect -f '{{index .Config.Labels "wpdock.domain"}}' "$c")
  [ -f "certs/renewal/$d.conf" ] && { echo "== $name: certificate exists"; continue; }
  echo "== ssl for $name ($d)"
  ./main ssl --name="$name" --email="$EMAIL" || { echo "== FAILED: $name"; fail=$((fail+1)); }
done

echo
echo "$fail failure(s)"
