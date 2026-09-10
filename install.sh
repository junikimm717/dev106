#!/bin/sh
# dev106 installer.
#
#   curl -fsSL https://raw.githubusercontent.com/junikimm717/dev106/master/install.sh | sh
#
# Installs the dev106 binary, then runs `dev106 pull`, which writes your config
# (asking which course you're taking) and downloads the course image.
#
# Environment overrides:
#   DEV106_VERSION            release tag to install (default: nightly)
#   DEV106_INSTALL_DIR        where to put the binary (default: ~/.local/bin)
#   DEV106_NO_MODIFY_PATH=1   don't touch your shell rc file
#   DEV106_SKIP_DOCKER=1      install even if Docker is missing or stopped
#   DEV106_SKIP_SETUP=1       don't run `dev106 pull` afterwards

set -eu

REPO="junikimm717/dev106"
VERSION="${DEV106_VERSION:-nightly}"
INSTALL_DIR="${DEV106_INSTALL_DIR:-$HOME/.local/bin}"
BASE_URL="https://github.com/$REPO/releases/download/$VERSION"

TMPDIR_=""
PATH_CHANGED=""
cleanup() { [ -n "$TMPDIR_" ] && rm -rf "$TMPDIR_"; }
trap cleanup EXIT INT TERM

say()  { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<EOF
dev106 installer

  curl -fsSL https://raw.githubusercontent.com/$REPO/master/install.sh | sh

Installs the dev106 binary, then runs \`dev106 pull\`, which writes your config
(asking which course you're taking) and downloads the course image.

Environment variables:
  DEV106_VERSION            release tag to install (default: nightly)
  DEV106_INSTALL_DIR        where to put the binary (default: ~/.local/bin)
  DEV106_NO_MODIFY_PATH=1   don't touch your shell rc file
  DEV106_SKIP_DOCKER=1      install even if Docker is missing or stopped
  DEV106_SKIP_SETUP=1       don't run \`dev106 pull\` afterwards

To pass these through a pipe, put them before \`sh\`:
  curl -fsSL <url> | DEV106_SKIP_SETUP=1 sh
EOF
}

# ---------------------------------------------------------------- platform

detect_target() {
  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Darwin) os=darwin ;;
    Linux)  os=linux ;;
    MINGW*|MSYS*|CYGWIN*)
      die "native Windows is not supported.
Install WSL2 (\`wsl --install\` in an admin PowerShell), open your Linux distro, and run this script there." ;;
    *) die "unsupported OS: $os" ;;
  esac

  case "$arch" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64)
      arch=amd64
      # An Apple Silicon Mac running this shell under Rosetta reports x86_64.
      # Installing the amd64 build there would work but run emulated.
      if [ "$os" = darwin ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = "1" ]; then
        arch=arm64
      fi
      ;;
    *) die "unsupported architecture: $arch" ;;
  esac

  OS="$os"
  TARGET="$os-$arch"
}

is_wsl() {
  [ -f /proc/version ] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null
}

# ---------------------------------------------------------------- docker

# dev106 talks to the Docker API socket directly; it never shells out to the
# `docker` CLI, and it does not read Docker contexts. It resolves its endpoint
# exactly the way this does: $DOCKER_HOST, else the default unix socket.
DEFAULT_DOCKER_HOST="unix:///var/run/docker.sock"

docker_endpoint() {
  if [ -n "${DOCKER_HOST:-}" ]; then
    echo "$DOCKER_HOST"
  else
    echo "$DEFAULT_DOCKER_HOST"
  fi
}

curl_has_unix_socket() {
  command -v curl >/dev/null 2>&1 && curl --help all 2>/dev/null | grep -q -- '--unix-socket'
}

# Ping the endpoint dev106 will use. Returns 0 = reachable, 1 = no socket,
# 2 = socket present but the daemon did not answer (not running, or denied).
docker_ping() {
  endpoint="$(docker_endpoint)"
  case "$endpoint" in
    unix://*)
      sock="${endpoint#unix://}"
      [ -S "$sock" ] || return 1
      if curl_has_unix_socket; then
        curl -fsS --max-time 5 --unix-socket "$sock" http://localhost/_ping >/dev/null 2>&1 || return 2
      fi
      return 0
      ;;
    *)
      # tcp:// or ssh:// — not worth probing here; trust the explicit setting.
      return 0
      ;;
  esac
}

# Docker Desktop and OrbStack put a socket at the default path. Colima,
# Rancher Desktop, Podman and rootless Docker instead register a *Docker
# context*, which the CLI honours but dev106 does not. Surfacing the context
# endpoint turns a confusing failure into a one-line fix.
suggest_docker_host() {
  command -v docker >/dev/null 2>&1 || return 1
  ep="$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null || true)"
  [ -n "$ep" ] || return 1
  if [ "$ep" = "$(docker_endpoint)" ]; then
    return 1
  fi
  echo "$ep"
}

docker_help() {
  ep="$(suggest_docker_host || true)"
  if [ -n "$ep" ]; then
    say "  Your active Docker context points at:"
    say "      $ep"
    say "  dev106 reads \$DOCKER_HOST and does not use Docker contexts, so tell it explicitly:"
    say "      export DOCKER_HOST=$ep"
    say "  (add that line to your shell rc to make it stick, then re-run this script)"
    return 0
  fi

  if is_wsl; then
    say "  You are on WSL. Pick one:"
    say "    - Docker Desktop (easiest): install it on Windows, then"
    say "      Settings > Resources > WSL Integration > enable this distro."
    say "      That exposes $DEFAULT_DOCKER_HOST inside the distro."
    say "    - Docker Engine inside the distro:"
    say "      curl -fsSL https://get.docker.com | sh"
    say "      sudo usermod -aG docker \"\$USER\" && sudo service docker start"
    say "      (log out and back in for the group change to apply)"
  elif [ "$OS" = darwin ]; then
    say "  Install and start one of:"
    say "    - Docker Desktop: https://docs.docker.com/desktop/install/mac-install/"
    say "    - OrbStack:       brew install --cask orbstack"
    say "  Both provide $DEFAULT_DOCKER_HOST."
    say "    - Colima:         brew install colima docker && colima start"
    say "      Colima does not, so also:"
    say "      export DOCKER_HOST=\"unix://\$HOME/.colima/default/docker.sock\""
  else
    say "  Install Docker Engine: https://docs.docker.com/engine/install/"
    say "  Then: sudo systemctl start docker && sudo usermod -aG docker \"\$USER\""
    say "  (log out and back in for the group change to apply)"
  fi
}

check_docker() {
  if [ "${DEV106_SKIP_DOCKER:-0}" = "1" ]; then
    DOCKER_OK=0
    return 0
  fi

  rc=0
  docker_ping || rc=$?
  if [ "$rc" = 0 ]; then
    DOCKER_OK=1
    return 0
  fi

  if [ "$rc" = 1 ]; then
    say "dev106 runs your coursework in Docker containers, but nothing is listening at" >&2
    say "  $(docker_endpoint)" >&2
  else
    say "A Docker socket exists at $(docker_endpoint), but the daemon did not respond." >&2
    say "It may be stopped, or your user may not have permission to use it." >&2
  fi
  docker_help >&2
  say "" >&2
  die "set up Docker, then re-run this script (or set DEV106_SKIP_DOCKER=1 to install the binary anyway)."
}

# ---------------------------------------------------------------- download

fetch() {
  # fetch <url> <dest>
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    die "need curl or wget to download dev106."
  fi
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    echo ""
  fi
}

verify() {
  # verify <file> <checksums> <name>
  want="$(grep -E "[[:space:]]\*?$3\$" "$2" 2>/dev/null | head -n1 | cut -d' ' -f1)"
  if [ -z "$want" ]; then
    warn "no checksum published for $3; skipping verification."
    return 0
  fi
  got="$(sha256_of "$1")"
  if [ -z "$got" ]; then
    warn "no sha256 tool found; skipping verification."
    return 0
  fi
  [ "$want" = "$got" ] || die "checksum mismatch for $3 (expected $want, got $got)."
  say "Checksum OK."
}

install_binary() {
  TMPDIR_="$(mktemp -d)"
  bin="dev106-$TARGET"

  fetch "$BASE_URL/$bin" "$TMPDIR_/$bin" \
    || die "could not download $BASE_URL/$bin — check that the '$VERSION' release exists."

  if fetch "$BASE_URL/checksums.txt" "$TMPDIR_/checksums.txt" 2>/dev/null; then
    verify "$TMPDIR_/$bin" "$TMPDIR_/checksums.txt" "$bin"
  else
    warn "could not download checksums.txt; skipping verification."
  fi

  chmod +x "$TMPDIR_/$bin"

  # Downloads via curl/wget are not quarantined, but be defensive on macOS.
  if [ "$OS" = darwin ] && command -v xattr >/dev/null 2>&1; then
    xattr -d com.apple.quarantine "$TMPDIR_/$bin" 2>/dev/null || true
  fi

  mkdir -p "$INSTALL_DIR" || die "could not create $INSTALL_DIR"
  # Overwriting a running binary in place fails; unlink first.
  rm -f "$INSTALL_DIR/dev106"
  mv "$TMPDIR_/$bin" "$INSTALL_DIR/dev106" \
    || die "could not write to $INSTALL_DIR. Re-run with DEV106_INSTALL_DIR=<dir>."

  "$INSTALL_DIR/dev106" --help >/dev/null 2>&1 \
    || die "the installed binary would not run. Wrong architecture? Please report this at https://github.com/$REPO/issues"

  say "Installed dev106 to $INSTALL_DIR/dev106"
}

# ---------------------------------------------------------------- PATH

rc_file() {
  case "$(basename "${SHELL:-sh}")" in
    zsh)  echo "${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash) if [ "$OS" = darwin ] && [ -f "$HOME/.bash_profile" ]; then
            echo "$HOME/.bash_profile"
          else
            echo "$HOME/.bashrc"
          fi ;;
    fish) echo "$HOME/.config/fish/config.fish" ;;
    *)    echo "$HOME/.profile" ;;
  esac
}

on_path() {
  case ":$PATH:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

setup_path() {
  on_path "$INSTALL_DIR" && return 0

  if [ "${DEV106_NO_MODIFY_PATH:-0}" = "1" ]; then
    warn "$INSTALL_DIR is not on your PATH. Add it yourself to use \`dev106\`."
    return 0
  fi

  rc="$(rc_file)"
  if [ -f "$rc" ] && grep -q 'added by the dev106 installer' "$rc" 2>/dev/null; then
    PATH_CHANGED="$rc"
    return 0
  fi

  mkdir -p "$(dirname "$rc")"
  {
    echo ""
    echo "# added by the dev106 installer"
    case "$rc" in
      *config.fish) echo "fish_add_path $INSTALL_DIR" ;;
      *)            echo "export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
    esac
  } >> "$rc"
  say "Added $INSTALL_DIR to your PATH in $rc"
  PATH_CHANGED="$rc"

  # So the setup step below can use the binary by name.
  PATH="$INSTALL_DIR:$PATH"
  export PATH
}

# ---------------------------------------------------------------- setup

# `curl ... | sh` leaves stdin pointed at the pipe, so the course picker would
# see no TTY and silently default to 6.181. Borrow the real terminal instead.
have_tty() {
  [ -e /dev/tty ] && (exec 3</dev/tty) 2>/dev/null
}

run_setup() {
  if [ "${DEV106_SKIP_SETUP:-0}" = "1" ] || [ "${DOCKER_OK:-0}" != "1" ]; then
    return 1
  fi

  if ! have_tty; then
    warn "no terminal available, so the course picker was skipped."
    say "  Run \`dev106 pull\` yourself to choose a course and download the image."
    return 1
  fi

  say ""
  say "Setting up your config and pulling the course image."
  say "(The images are several GB; this can take a while on slow wifi.)"
  say ""

  if "$INSTALL_DIR/dev106" pull < /dev/tty; then
    return 0
  fi

  say ""
  warn "\`dev106 pull\` did not finish. The binary is installed; re-run \`dev106 pull\` to retry."
  return 1
}

# ---------------------------------------------------------------- main

# A leftover `go install` copy in ~/go/bin is the classic way for someone to
# "install" dev106 and then keep running an old build without noticing.
check_shadow() {
  other="$(PATH="$ORIG_PATH" command -v dev106 2>/dev/null || true)"
  [ -n "$other" ] || return 0
  if [ "$other" = "$INSTALL_DIR/dev106" ]; then
    return 0
  fi
  warn "a different dev106 is already earlier on your PATH:"
  say "      $other"
  say "  That one will keep winning. Either delete it:"
  say "      rm $other"
  say "  or install over it instead:"
  say "      DEV106_INSTALL_DIR=$(dirname "$other") sh install.sh"
}

main() {
  for arg in "$@"; do
    case "$arg" in
      -h|--help) usage; exit 0 ;;
      *) die "unknown argument: $arg (try --help)" ;;
    esac
  done

  ORIG_PATH="$PATH"

  detect_target
  say "Installing dev106 ($TARGET, $VERSION)..."

  check_docker
  install_binary
  setup_path
  check_shadow

  if run_setup; then
    say ""
    say "Done. To start working:"
    [ -n "$PATH_CHANGED" ] && say "  0. restart your shell (or run: . $PATH_CHANGED)"
    say "  1. cd into a course assignment repo"
    say "  2. dev106"
  else
    say ""
    say "Next steps:"
    [ -n "$PATH_CHANGED" ] && say "  0. restart your shell (or run: . $PATH_CHANGED)"
    say "  1. cd into a course assignment repo"
    say "  2. dev106 pull"
    say "  3. dev106"
  fi
}

main "$@"
