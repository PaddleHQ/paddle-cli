#!/bin/sh
# Installs the Paddle CLI from its GitHub releases, verifying the checksum first.
#   curl -fsSL https://raw.githubusercontent.com/PaddleHQ/paddle-cli/main/install.sh | sh -s -- --help

set -eu

REPO="PaddleHQ/paddle-cli"
BINARY="paddle"
RELEASES_URL="https://github.com/${REPO}/releases"

say() {
	printf '%s\n' "$*"
}

fail() {
	printf 'install.sh: error: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<EOF
Install the Paddle CLI from ${RELEASES_URL}.

Usage: install.sh [--version <version>] [--dir <directory>]

  --version <version>  Install this version, e.g. v1.2.0 (default: latest).
                       Also set with PADDLE_INSTALL_VERSION.
  --dir <directory>    Install into this directory. Also set with
                       PADDLE_INSTALL_DIR. Default: /usr/local/bin if writable,
                       otherwise ~/.local/bin.
  -h, --help           Show this help.
EOF
}

detect_os() {
	case "$(uname -s)" in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	MINGW* | MSYS* | CYGWIN*)
		fail "on Windows, download the .zip archive from ${RELEASES_URL}/latest instead"
		;;
	*) fail "unsupported operating system: $(uname -s)" ;;
	esac
}

detect_arch() {
	arch="$(uname -m)"
	# A shell running under Rosetta reports x86_64 on Apple silicon; the
	# native arm64 build is the better choice there.
	if [ "$arch" = x86_64 ] && [ "$(uname -s)" = Darwin ] &&
		[ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" = 1 ]; then
		arch=arm64
	fi
	case "$arch" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "unsupported architecture: $arch" ;;
	esac
}

download() {
	url="$1"
	out="$2"
	if command -v curl >/dev/null 2>&1; then
		curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
			--output "$out" "$url"
	elif command -v wget >/dev/null 2>&1; then
		# BusyBox wget (Alpine) has no --https-only, so only GNU wget gets it.
		if wget --help 2>&1 | grep -q -- --https-only; then
			wget -q --https-only -O "$out" "$url"
		else
			wget -q -O "$out" "$url"
		fi
	else
		fail "curl or wget is required"
	fi
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	elif command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 "$1" | sed 's/^.*= //'
	else
		fail "sha256sum, shasum or openssl is required to verify the download"
	fi
}

default_dir() {
	if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
		echo /usr/local/bin
	else
		echo "${HOME}/.local/bin"
	fi
}

on_path() {
	case ":${PATH}:" in
	*":$1:"*) return 0 ;;
	*) return 1 ;;
	esac
}

main() {
	version="${PADDLE_INSTALL_VERSION:-}"
	dir="${PADDLE_INSTALL_DIR:-}"

	while [ $# -gt 0 ]; do
		case "$1" in
		--version)
			[ $# -ge 2 ] || fail "--version needs a value"
			version="$2"
			shift 2
			;;
		--version=*)
			version="${1#*=}"
			shift
			;;
		--dir)
			[ $# -ge 2 ] || fail "--dir needs a value"
			dir="$2"
			shift 2
			;;
		--dir=*)
			dir="${1#*=}"
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*) fail "unknown option: $1 (see --help)" ;;
		esac
	done

	os="$(detect_os)"
	arch="$(detect_arch)"

	# Release tags are v-prefixed; accept the version with or without the v.
	if [ -n "$version" ]; then
		base_url="${RELEASES_URL}/download/v${version#v}"
	else
		base_url="${RELEASES_URL}/latest/download"
	fi

	tmp="$(mktemp -d 2>/dev/null || mktemp -d -t paddle-install)"
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 1' HUP INT TERM

	download "${base_url}/checksums.txt" "${tmp}/checksums.txt" ||
		fail "couldn't download checksums.txt from ${base_url}${version:+ (does ${version} exist?)}"

	# checksums.txt lists every archive in the release, so it also gives the
	# exact archive name, version included, for this platform.
	pattern=" ${BINARY}_[^ ]*_${os}_${arch}\\.tar\\.gz\$"
	matches="$(grep -c -- "$pattern" "${tmp}/checksums.txt" || true)"
	[ "$matches" = 1 ] || fail "expected one ${os}/${arch} archive in checksums.txt, found ${matches:-0}"
	line="$(grep -- "$pattern" "${tmp}/checksums.txt")"
	expected="${line%% *}"
	archive="${line##* }"
	release="${archive#"${BINARY}_"}"
	release="v${release%"_${os}_${arch}.tar.gz"}"

	say "Downloading ${BINARY} ${release} for ${os}/${arch}..."
	download "${RELEASES_URL}/download/${release}/${archive}" "${tmp}/${archive}" ||
		fail "couldn't download ${archive}"

	actual="$(sha256_of "${tmp}/${archive}")"
	[ "$actual" = "$expected" ] ||
		fail "checksum mismatch for ${archive}: expected ${expected}, got ${actual}"

	mkdir "${tmp}/extract"
	tar -xzf "${tmp}/${archive}" -C "${tmp}/extract"
	[ -f "${tmp}/extract/${BINARY}" ] || fail "${archive} doesn't contain ${BINARY}"

	[ -n "$dir" ] || dir="$(default_dir)"
	# Shells don't expand ~ in --dir=~/bin or a quoted PADDLE_INSTALL_DIR.
	case "$dir" in
	\~ | \~/*) dir="${HOME}${dir#\~}" ;;
	*) ;;
	esac
	mkdir -p "$dir" 2>/dev/null || fail "couldn't create ${dir}"
	[ -w "$dir" ] || fail "${dir} isn't writable; choose another with --dir, or re-run with sudo"
	[ ! -d "${dir}/${BINARY}" ] || fail "${dir}/${BINARY} is a directory"

	# Copy then rename, so an existing binary is replaced in one step. Removing
	# a leftover first stops cp writing through a stale file or symlink.
	staged="${dir}/.${BINARY}.new"
	rm -f "$staged"
	trap 'rm -rf "$tmp"; rm -f "$staged"' EXIT
	cp "${tmp}/extract/${BINARY}" "$staged"
	chmod 755 "$staged"
	mv -f "$staged" "${dir}/${BINARY}"

	say "Installed ${BINARY} ${release} to ${dir}/${BINARY}"
	if ! on_path "$dir"; then
		say ""
		say "${dir} isn't on your PATH. Add it to your shell profile:"
		say "  export PATH=\"${dir}:\$PATH\""
	fi
	say "Run '${BINARY} --help' to get started."
}

# Everything runs from main, so a partially downloaded script does nothing.
main "$@"
