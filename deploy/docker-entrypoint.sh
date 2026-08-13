#!/bin/sh
set -eu

image_runtime=${DEEIX_IMAGE_RUNTIME_DIR:-/app/image-runtime}
runtime=${UPDATE_RUNTIME_DIR:-/app/runtime}
releases="$runtime/releases"
current="$runtime/current"

valid_version() {
  printf '%s' "$1" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
}

image_version=$(tr -d '\r\n' < "$image_runtime/VERSION")
valid_version "$image_version"
image_digest=$(tr -d '\r\n' < "$image_runtime/IMAGE_DIGEST")
printf '%s' "$image_digest" | grep -Eq '^[0-9a-f]{64}$'
image_release="image-$image_version-$image_digest"
case "$image_runtime:$runtime" in
  /*:/*) ;;
  *) echo "runtime paths must be absolute" >&2; exit 1 ;;
esac
[ "$image_runtime" != / ] && [ "$runtime" != / ]
mkdir -p "$releases"

valid_release() {
  root=$1
  [ -d "$root" ] \
    && [ ! -L "$root" ] \
    && [ -x "$root/deeix-chat" ] \
    && [ -f "$root/frontend/out/index.html" ] \
    && [ "$(tr -d '\r\n' < "$root/VERSION" 2>/dev/null || true)" = "$image_version" ]
}

seed_image_release() {
  target="$releases/$image_release"
  if valid_release "$target" \
    && [ -f "$target/IMAGE_DIGEST" ] \
    && [ "$(tr -d '\r\n' < "$target/IMAGE_DIGEST")" = "$image_digest" ]; then
    return
  fi
  if [ -e "$target" ] || [ -L "$target" ]; then
    echo "image release path is invalid" >&2
    exit 1
  fi

  stage="$runtime/.$image_release-$$"
  rm -rf "$stage"
  mkdir "$stage"
  cp -a "$image_runtime/." "$stage/"
  valid_release "$stage"
  [ "$(tr -d '\r\n' < "$stage/IMAGE_DIGEST")" = "$image_digest" ]
  mv "$stage" "$target"
}

activate() {
  release=$1
  link="$runtime/.current-$$"
  ln -s "releases/$release" "$link"
  mv -Tf "$link" "$current"
  current_release="releases/$release"
}

prune_image_releases() {
  for path in "$releases"/image-*; do
    [ -e "$path" ] || continue
    name=${path##*/}
    [ "$name" = "$image_release" ] && continue
    [ "releases/$name" = "$current_release" ] && continue
    printf '%s' "$name" | grep -Eq '^image-([0-9]+\.){2}[0-9]+-[0-9a-f]{64}$' || continue
    [ -d "$path" ] && [ ! -L "$path" ] || continue
    rm -rf "$path" || echo "failed to prune stale image release: $name" >&2
  done
}

seed_image_release
if [ -e "$current" ] && [ ! -L "$current" ]; then
  echo "current release path must be a symlink" >&2
  exit 1
fi

current_version=""
current_release=""
if [ -L "$current" ]; then
  current_release=$(readlink "$current")
  printf '%s' "$current_release" | grep -Eq '^releases/(([0-9]+\.){2}[0-9]+|image-([0-9]+\.){2}[0-9]+-[0-9a-f]{64})$' || {
    echo "current release target is invalid" >&2
    exit 1
  }
fi
if [ -n "$current_release" ] && [ -f "$current/VERSION" ]; then
  current_version=$(tr -d '\r\n' < "$current/VERSION")
  valid_version "$current_version" || current_version=""
fi

if [ -z "$current_version" ]; then
  activate "$image_release"
elif [ "$current_version" != "$image_version" ]; then
  newest=$(printf '%s\n%s\n' "$current_version" "$image_version" | sort -V | tail -n 1)
  if [ "$newest" = "$image_version" ]; then
    activate "$image_release"
  fi
elif printf '%s' "$current_release" | grep -Eq '^releases/image-'; then
  [ "$current_release" = "releases/$image_release" ] || activate "$image_release"
fi
prune_image_releases

export FRONTEND_DIST_DIR="$current/frontend/out"
export UPDATE_RUNTIME_DIR="$runtime"
exec "$current/deeix-chat" "$@"
