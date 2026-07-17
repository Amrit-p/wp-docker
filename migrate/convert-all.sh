#!/bin/sh
cd "$(dirname "$0")/.." || exit 1

PASS="$1"
OLD="${2:-$HOME/wp-stack-docker}"
[ -n "$PASS" ] || { echo "usage: bash migrate/convert-all.sh '<mariadb-root-password>' [old-prefix]"; exit 1; }
[ -d "$OLD/sites" ] || { echo "$OLD/sites does not exist"; exit 1; }

for name in $(ls "$OLD/sites"); do
  docker inspect "wpdock-$name" >/dev/null 2>&1 && { echo "== $name: already converted"; continue; }
  docker ps -q -f "name=^wp_$name$" | grep -q . || { echo "== $name: old container not running - skipped"; continue; }
  echo "== converting $name"
  wp-dock site-convert --old-prefix="$OLD" --name="$name" --root-password="$PASS" --yes || echo "== FAILED: $name"
done 2>&1 | tee convert.log

echo
grep -c '^== FAILED' convert.log | xargs -I{} echo "{} failure(s) - grep FAILED convert.log"
