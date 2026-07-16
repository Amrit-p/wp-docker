#!/bin/sh
cd "$(dirname "$0")/.." || exit 1

PORT="${1:-8080}"
bad=0

printf '%-35s %-9s %-5s %-6s %-6s %s\n' SITE STATE ROOT LOGIN HTTPS VERDICT
for c in $(docker ps -a --filter label=wpdock.managed=true --format '{{.Names}}' | sort); do
  name=${c#wpdock-}
  d=$(docker inspect -f '{{index .Config.Labels "wpdock.domain"}}' "$c")
  t=$(docker inspect -f '{{index .Config.Labels "wpdock.type"}}' "$c")
  state=$(docker inspect -f '{{.State.Status}}' "$c")

  root=$(curl -s -o /dev/null -w '%{http_code}' -m 15 -H "Host: $d" "http://127.0.0.1:$PORT/")
  login=-
  [ "$t" = wordpress ] && login=$(curl -s -o /dev/null -w '%{http_code}' -m 15 -H "Host: $d" "http://127.0.0.1:$PORT/wp-login.php")
  https=-
  [ -f "certs/renewal/$d.conf" ] && https=$(curl -sk -o /dev/null -w '%{http_code}' -m 15 --resolve "$d:443:127.0.0.1" "https://$d/")

  verdict=ok
  [ "$state" = running ] || verdict=BAD
  case $root in 200|301|302) ;; *) verdict=BAD ;; esac
  case $login in -|200|301|302) ;; *) verdict=BAD ;; esac
  case $https in -|200|301|302) ;; *) verdict=BAD ;; esac
  [ "$verdict" = BAD ] && bad=$((bad+1))

  printf '%-35s %-9s %-5s %-6s %-6s %s\n' "$name" "$state" "$root" "$login" "$https" "$verdict"
done

echo
echo "$bad site(s) need attention"
