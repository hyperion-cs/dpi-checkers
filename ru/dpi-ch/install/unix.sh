#!/usr/bin/env bash
set -euo pipefail

REPO="hyperion-cs/dpi-checkers"

BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
APP_DIR="${APP_DIR:-$BIN_DIR}"

require() {
	command -v "$1" >/dev/null || {
		echo "$1 is not installed" >&2
		exit 1
	}
}

require curl
require unzip

uname_os="$(uname -o 2>/dev/null || uname -s)"

case "$uname_os" in
Android) os="android" ;;
Darwin) os="darwin" ;;
GNU/Linux | Linux)
	os="linux"
	if [[ -n "${TERMUX_VERSION:-}" ]]; then
		os="android"
	fi
	;;
*)
	echo "Unsupported OS: $uname_os" >&2
	exit 1
	;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
*)
	echo "Unsupported architecture: $(uname -m)" >&2
	exit 1
	;;
esac

platform="${os}-${arch}"
echo "Platform detected: $platform"

mkdir -p -- "$APP_DIR" "$BIN_DIR"
APP_DIR="$(CDPATH= cd -- "$APP_DIR" && pwd -P)"
BIN_DIR="$(CDPATH= cd -- "$BIN_DIR" && pwd -P)"
BIN_PATH="$APP_DIR/dpich"
LINK_PATH="$BIN_DIR/dpich"

if [[ -d "$BIN_PATH" || -d "$LINK_PATH" ]]; then
	echo "Installation destination is a directory: $BIN_PATH or $LINK_PATH" >&2
	exit 1
fi

tmp_dir="$(mktemp -d "$APP_DIR/.dpich-install.XXXXXX")"
tmp_json="$tmp_dir/release.json"
tmp_zip="$tmp_dir/archive.zip"
staged_bin="$tmp_dir/dpich"
link_tmp_dir=""

cleanup() {
	rm -rf -- "$tmp_dir"
	if [[ -n "$link_tmp_dir" ]]; then
		rm -rf -- "$link_tmp_dir"
	fi
}
trap cleanup EXIT

echo "Install directory prepared: $APP_DIR"

if [[ -n "${ASSET_URL:-}" ]]; then
	asset_url="$ASSET_URL"
	echo "Using asset URL from ASSET_URL: $asset_url"
else
	curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" -o "$tmp_json"
	echo "Latest release info fetched: https://github.com/${REPO}/releases/latest"

	asset_url="$(
		grep -Eo '"browser_download_url":[[:space:]]*"[^"]+' "$tmp_json" |
			sed -E 's/^"browser_download_url":[[:space:]]*"//' |
			grep -E -- "-${platform}\.zip$" |
			head -n 1 ||
			true
	)"
fi

if [[ -z "$asset_url" ]]; then
	echo "No release archive found for platform: $platform" >&2
	exit 1
fi

curl -fL "$asset_url" -o "$tmp_zip"
echo "Archive downloaded: $asset_url"

unzip -o "$tmp_zip" dpich -d "$tmp_dir" >/dev/null

if [[ ! -f "$staged_bin" || ! -s "$staged_bin" || -L "$staged_bin" ]]; then
	echo "Archive contains no usable dpich binary" >&2
	exit 1
fi

chmod +x "$staged_bin"
if [[ "$BIN_PATH" != "$LINK_PATH" ]]; then
	link_tmp_dir="$(mktemp -d "$BIN_DIR/.dpich-install.XXXXXX")"
	ln -s -- "$BIN_PATH" "$link_tmp_dir/dpich"
fi

mv -f -- "$staged_bin" "$BIN_PATH"

if [[ -n "$link_tmp_dir" ]]; then
	mv -f -- "$link_tmp_dir/dpich" "$LINK_PATH"
	echo "Symlink created: $LINK_PATH -> $BIN_PATH"
fi

echo "Run:"
if [[ "$(command -v dpich)" -ef "$BIN_PATH" ]]; then
	echo "  dpich"
else
	printf '  %q\n' "$BIN_PATH"
	echo
	echo "To run as 'dpich', put its directory first in PATH by adding this to your shell config."
	printf '  export PATH=%q:"$PATH"\n' "$BIN_DIR"
fi

echo
echo "Successfully installed: $BIN_PATH"
