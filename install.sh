#!/bin/sh
# Install the latest sheen release from GitHub.
# Usage: curl -fsSL https://raw.githubusercontent.com/hangarbay/sheen/master/install.sh | sh
set -eu

repo="hangarbay/sheen"
dest="${SHEEN_INSTALL_DIR:-/usr/local/bin}"

case "$(uname -s)" in
	Darwin) goos="darwin" ;;
	Linux) goos="linux" ;;
	*) echo "sheen installer: unsupported OS '$(uname -s)'" >&2; exit 1 ;;
esac
case "$(uname -m)" in
	arm64 | aarch64) goarch="arm64" ;;
	x86_64 | amd64) goarch="amd64" ;;
	*) echo "sheen installer: unsupported arch '$(uname -m)'" >&2; exit 1 ;;
esac

command -v curl >/dev/null 2>&1 ||
	{ echo "sheen installer: curl is required" >&2; exit 1; }

tag="$(curl -fsSL -o /dev/null -w '%{url_effective}' \
	"https://github.com/${repo}/releases/latest" | sed 's|.*/tag/||')"
[ -n "${tag}" ] ||
	{ echo "sheen installer: could not determine latest release" >&2; exit 1; }

version="${tag#v}"
tarball="sheen_${version}_${goos}_${goarch}.tar.gz"
url="https://github.com/${repo}/releases/download/${tag}/${tarball}"

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
curl -fsSL "${url}" -o "${tmp}/${tarball}"
tar -xzf "${tmp}/${tarball}" -C "${tmp}"

if ! mkdir -p "${dest}" 2>/dev/null || [ ! -w "${dest}" ]; then
	dest="${HOME}/.local/bin"
	mkdir -p "${dest}"
	echo "sheen installer: default destination not writable, using ${dest}"
fi
install -m 0755 "${tmp}/sheen" "${dest}/sheen"
echo "sheen installer: installed ${dest}/sheen (${tag})"
case ":${PATH}:" in
	*":${dest}:"*) ;;
	*) echo "sheen installer: note: ${dest} is not in your PATH" ;;
esac
