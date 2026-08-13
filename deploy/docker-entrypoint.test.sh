#!/bin/sh
set -eu

entrypoint=$(CDPATH= cd -- "$(dirname "$0")" && pwd)/docker-entrypoint.sh
root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT

make_release() {
  target=$1
  version=$2
  label=$3
  digest=$4
  mkdir -p "$target/frontend/out"
  printf '%s\n' "$version" > "$target/VERSION"
  printf '%s\n' "$digest" > "$target/IMAGE_DIGEST"
  printf '<!doctype html>%s\n' "$label" > "$target/frontend/out/index.html"
  printf '#!/bin/sh\nprintf "%%s\\n" %s\n' "$label" > "$target/deeix-chat"
  chmod 755 "$target/deeix-chat"
}

same_version="$root/same-version"
make_release "$same_version/image" 0.4.16 image-new "$(printf '1%.0s' $(seq 1 64))"
make_release "$same_version/runtime/releases/image-0.4.16-$(printf '0%.0s' $(seq 1 64))" 0.4.16 image-old "$(printf '0%.0s' $(seq 1 64))"
ln -s "releases/image-0.4.16-$(printf '0%.0s' $(seq 1 64))" "$same_version/runtime/current"
result=$(DEEIX_IMAGE_RUNTIME_DIR="$same_version/image" UPDATE_RUNTIME_DIR="$same_version/runtime" sh "$entrypoint")
[ "$result" = image-new ]
[ "$("$same_version/runtime/current/deeix-chat")" = image-new ]
[ ! -e "$same_version/runtime/releases/image-0.4.16-$(printf '0%.0s' $(seq 1 64))" ]

online="$root/online"
make_release "$online/image" 0.4.16 image-baseline "$(printf '2%.0s' $(seq 1 64))"
make_release "$online/runtime/releases/0.4.16" 0.4.16 online-stable "$(printf '3%.0s' $(seq 1 64))"
ln -s releases/0.4.16 "$online/runtime/current"
result=$(DEEIX_IMAGE_RUNTIME_DIR="$online/image" UPDATE_RUNTIME_DIR="$online/runtime" sh "$entrypoint")
[ "$result" = online-stable ]
[ "$(readlink "$online/runtime/current")" = releases/0.4.16 ]

echo "docker entrypoint checks passed"
