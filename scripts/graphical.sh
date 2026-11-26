#!/bin/sh
# Build and launch the graphical clients on the fictional demo profile.
#   scripts/graphical.sh web       browser client (opens your default browser)
#   scripts/graphical.sh desktop   Qt desktop client (Community records only)
# Set REBUILD=1 to rebuild the browser assets. Set NO_PRO=1 to skip Professional.
# Run from anywhere; paths are resolved from this script.
set -eu

mode=${1:-web}
case "$mode" in web | desktop) ;; *) echo "usage: $0 web|desktop" >&2; exit 2 ;; esac

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
pro=$root/../tidecontacts-pro
cd "$root"

# Everything the demo needs lives in one place and never touches your real data.
ext=${TIDECONTACTS_EXTENSIONS_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/tidecontacts-graphical-ext}
export TIDECONTACTS_EXTENSIONS_DIR=$ext
export TIDECONTACTS_DEV_ENTITLEMENTS='*' # development only, not a licence

echo "== Building"
if [ ! -f web/dist/index.html ] || [ "${REBUILD:-}" = 1 ]; then
	(cd web && npm ci && npm run build)
fi
go build -buildvcs=false -o bin/tidecontacts ./cmd/tidecontacts
go build -tags webdist -buildvcs=false -o bin/tidecontacts-web ./cmd/tidecontacts-web

if [ "${NO_PRO:-}" != 1 ] && [ -d "$pro" ]; then
	echo "== Professional extension (development entitlements)"
	mkdir -p "$ext/pro-sales"
	(cd "$pro" && go build -buildvcs=false -o "$ext/pro-sales/tidecontacts-pro" ./cmd/tidecontacts-pro)
	cp "$pro/extension.toml" "$ext/pro-sales/"
	bin/tidecontacts --demo extensions enable pro-sales --yes >/dev/null
	# Seeding is refused once the demo already has data, which is fine.
	bin/tidecontacts --demo sales demo-seed --yes >/dev/null 2>&1 || true
else
	echo "== Professional skipped; Sales, Business and Automation will not appear"
fi

if [ "$mode" = desktop ]; then
	echo "== Desktop"
	mkdir -p bin/desktop
	(cd bin/desktop && qmake6 ../../desktop/tidecontacts-desktop.pro >/dev/null && make -j4 >/dev/null)
	exec bin/desktop/tidecontacts-desktop --profile demo --server "$root/bin/tidecontacts-web"
fi

# The server prints one line, the session URL. Show it and open the browser.
log=$(mktemp)
bin/tidecontacts-web --profile demo >"$log" &
server=$!
trap 'kill "$server" 2>/dev/null; rm -f "$log"' EXIT INT TERM
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
	[ -s "$log" ] && break
	sleep 0.25
done
url=$(head -n 1 "$log")
[ -n "$url" ] || { echo "The server did not start." >&2; exit 1; }
echo
echo "Open: $url"
echo "(Ctrl+C stops the server)"
command -v xdg-open >/dev/null 2>&1 && xdg-open "$url" >/dev/null 2>&1 || true
wait "$server"
