#!/bin/sh
set -eu

fail() { printf 'rian: %s\n' "$*" >&2; exit 1; }
version=
install_dir=${RIAN_INSTALL_DIR:-"$HOME/.local/share/rian"}
bin_dir=${RIAN_BIN_DIR:-"$HOME/.local/bin"}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || fail '--version needs a value'; version=$2; shift 2 ;;
    --help) printf 'Usage: install.sh [--version X.Y.Z]\nSet RIAN_INSTALL_DIR and RIAN_BIN_DIR to change installation paths.\n'; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
done
case "$install_dir:$bin_dir" in /*:/*) ;; *) fail 'installation paths must be absolute' ;; esac
case "$(uname -s)" in Darwin) platform=darwin ;; Linux) platform=linux ;; *) fail 'use the Windows ZIP from GitHub Releases' ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) fail 'supported architectures: x64 and ARM64' ;; esac
command -v curl >/dev/null 2>&1 || fail 'curl is required'
command -v tar >/dev/null 2>&1 || fail 'tar is required'
base=https://github.com/FatalMistake02/rian/releases
if [ -z "$version" ]; then version=$(curl -fsSL --proto '=https' --tlsv1.2 "$base/latest/download/VERSION"); fi
version=${version#v}
printf '%s\n' "$version" | LC_ALL=C grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || fail 'invalid release version'
name=rian_${version}_${platform}_${arch}.tar.gz
tmp=$(mktemp -d) || fail 'cannot create temporary directory'
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
curl -fsSL --proto '=https' --tlsv1.2 "$base/download/v$version/$name" -o "$tmp/$name"
curl -fsSL --proto '=https' --tlsv1.2 "$base/download/v$version/SHA256SUMS" -o "$tmp/SHA256SUMS"
expected=$(awk -v file="$name" '$2 == file {print $1}' "$tmp/SHA256SUMS")
[ "${#expected}" -eq 64 ] || fail 'missing or ambiguous archive checksum'
case "$expected" in *[!0-9a-f]*) fail 'invalid archive checksum' ;; esac
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$name");
elif command -v shasum >/dev/null 2>&1; then actual=$(shasum -a 256 "$tmp/$name");
else fail 'sha256sum or shasum is required'; fi
[ "${actual%% *}" = "$expected" ] || fail 'archive checksum mismatch'
# Accept only the expected regular files. Never extract archive-supplied paths.
tar -tzf "$tmp/$name" > "$tmp/entries"
[ "$(wc -l < "$tmp/entries" | tr -d ' ')" = 3 ] || fail 'invalid archive contents'
for file in rian LICENSE THIRD_PARTY_NOTICES.txt; do
  [ "$(grep -Fxc "$file" "$tmp/entries")" = 1 ] || fail 'invalid archive contents'
  tar -xOzf "$tmp/$name" "$file" > "$tmp/$file"
  [ -s "$tmp/$file" ] || fail 'empty archive file'
done
chmod 755 "$tmp/rian"
[ "$("$tmp/rian" --version)" = "rian $version" ] || fail 'executable version mismatch'
mkdir -p "$install_dir" "$bin_dir"
if [ -e "$bin_dir/rian" ] || [ -L "$bin_dir/rian" ]; then
  [ -L "$bin_dir/rian" ] && [ "$(readlink "$bin_dir/rian")" = "$install_dir/rian" ] || fail "$bin_dir/rian already belongs to another installation"
fi
# Stage on the destination filesystem so replacing a running Unix binary is atomic.
stage=$(mktemp -d "$install_dir/.install-XXXXXX")
cp "$tmp/rian" "$stage/rian"
cp "$tmp/LICENSE" "$install_dir/LICENSE"
cp "$tmp/THIRD_PARTY_NOTICES.txt" "$install_dir/THIRD_PARTY_NOTICES.txt"
mv -f "$stage/rian" "$install_dir/rian"
rmdir "$stage"
ln -sfn "$install_dir/rian" "$bin_dir/rian"
printf 'Installed Rian %s to %s/rian\n' "$version" "$bin_dir"
case ":$PATH:" in *":$bin_dir:"*) ;; *) printf 'Add %s to your PATH to run rian.\n' "$bin_dir" ;; esac
