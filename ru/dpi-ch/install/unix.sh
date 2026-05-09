#!/usr/bin/env bash
set -euo pipefail

REPO="hyperion-cs/dpi-checkers"

APP_DIR="${APP_DIR:-$HOME/.local/dpi-ch}"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
BIN_PATH="$APP_DIR/dpich"
LINK_PATH="$BIN_DIR/dpich"

require() {
	command -v "$1" >/dev/null || {
		echo "$1 is not installed" >&2
		exit 1
	}
}

require curl
require unzip

case "$(uname -s)" in
Darwin) os="darwin" ;;
Linux) os="linux" ;;
*)
	echo "Unsupported OS: $(uname -s)" >&2
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

tmp_dir="$(mktemp -d)"
tmp_json="$tmp_dir/release.json"
tmp_zip="$tmp_dir/archive.zip"

cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT

mkdir -p "$APP_DIR" "$BIN_DIR"
echo "Install directory prepared: $APP_DIR"
echo "Binary link directory prepared: $BIN_DIR"

curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" -o "$tmp_json"
echo "Latest release info fetched: https://github.com/${REPO}/releases/latest"

asset_url="$(
	grep -Eo '"browser_download_url":[[:space:]]*"[^"]+' "$tmp_json" |
		sed -E 's/^"browser_download_url":[[:space:]]*"//' |
		grep -E -- "-${platform}\.zip$" |
		head -n 1 ||
		true
)"

if [[ -z "$asset_url" ]]; then
	echo "No release archive found for platform: $platform" >&2
	exit 1
fi

curl -fL "$asset_url" -o "$tmp_zip"
echo "Archive downloaded: $asset_url"

unzip -o "$tmp_zip" -d "$APP_DIR" >/dev/null
echo "Archive extracted to: $APP_DIR"

if [[ ! -f "$BIN_PATH" ]]; then
	echo "Binary not found after extraction: $BIN_PATH" >&2
	exit 1
fi

chmod +x "$BIN_PATH"
echo "Binary made executable: $BIN_PATH"

ln -sf "$BIN_PATH" "$LINK_PATH"
echo "Symlink created: $LINK_PATH -> $BIN_PATH"

case ":$PATH:" in
*":$BIN_DIR:"*)
	echo "PATH already contains: $BIN_DIR"
	echo "Run:"
	echo "  dpich"
	;;
*)
	echo "PATH does not contain: $BIN_DIR"
	echo
	echo "Run without PATH:"
	echo "  ${BIN_PATH/#$HOME/~}"
	echo
	echo "To run simply as 'dpich', add this to your shell config:"
	echo
	echo "  export PATH=\"$BIN_DIR:\$PATH\""
	;;
esac

echo
echo "Successfully installed: $BIN_PATH"
echo "Symlink: $LINK_PATH"
