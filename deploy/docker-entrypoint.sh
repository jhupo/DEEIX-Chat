#!/bin/sh
set -eu

image_runtime=/app/image-runtime
runtime=/app/runtime
releases="$runtime/releases"
current="$runtime/current"

valid_version() {
  printf '%s' "$1" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
}

image_version=$(tr -d '\r\n' < "$image_runtime/VERSION")
valid_version "$image_version"
mkdir -p "$releases"

seed_image_release() {
  target="$releases/$image_version"
  if [ -d "$target" ] && { [ ! -x "$target/deeix-chat" ] || [ ! -f "$target/frontend/out/index.html" ] || [ "$(tr -d '\r\n' < "$target/VERSION" 2>/dev/null || true)" != "$image_version" ]; }; then
    rm -rf "$target"
  fi
  if [ ! -d "$target" ]; then
    stage="$runtime/.image-$image_version-$$"
    rm -rf "$stage"
    mkdir "$stage"
    cp -a "$image_runtime/." "$stage/"
    test -x "$stage/deeix-chat"
    test -f "$stage/frontend/out/index.html"
    mv "$stage" "$target"
  fi
}

activate() {
  version=$1
  link="$runtime/.current-$$"
  ln -s "releases/$version" "$link"
  mv -Tf "$link" "$current"
}

seed_image_release
if [ -e "$current" ] && [ ! -L "$current" ]; then
  echo "current release path must be a symlink" >&2
  exit 1
fi

current_version=""
if [ -L "$current" ] && [ -f "$current/VERSION" ]; then
  current_version=$(tr -d '\r\n' < "$current/VERSION")
  valid_version "$current_version" || current_version=""
fi

if [ -z "$current_version" ]; then
  activate "$image_version"
elif [ "$current_version" != "$image_version" ]; then
  newest=$(printf '%s\n%s\n' "$current_version" "$image_version" | sort -V | tail -n 1)
  if [ "$newest" = "$image_version" ]; then
    activate "$image_version"
  fi
fi

export FRONTEND_DIST_DIR="$current/frontend/out"
export UPDATE_RUNTIME_DIR="$runtime"
exec "$current/deeix-chat" "$@"
