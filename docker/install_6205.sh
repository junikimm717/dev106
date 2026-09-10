#!/usr/bin/env bash
# =============================================================================
# install_6205.sh - MIT 6.205 open-source FPGA toolchain for a Linux VM/container
# =============================================================================
#
# Installs everything 6.205 needs locally EXCEPT Vivado. Builds go through the
# course's remote build client (lab-bc), so the ~90 GB suite is never needed:
#
#   * Icarus Verilog >= 12  distro package, or built from source if too old
#   * openFPGALoader        built from a pinned upstream tag (as the course does)
#   * USB access            upstream udev rules + plugdev group membership
#   * Python venv           cocotb (course-pinned), pyserial, lab-bc, vicoco
#   * GUI tools (optional)  gtkwave, pulseview
#
# Idempotent: every step inspects the current state first and only touches
# what is missing or wrong. A second run on a finished machine changes nothing.
#
# Failure handling: each step runs in isolation. A failure reports the exact
# command and line; required steps stop the run with a summary; optional steps
# are reported and skipped; everything is logged. Re-running resumes.
#
# Needs: Linux, bash >= 4.4, coreutils/awk/sed/grep, and one of
#        apt-get / dnf / pacman / zypper. Run it as your normal user (it uses
#        sudo only when needed) or as root (e.g. in a Dockerfile).
#
# Usage: ./install_6205.sh [options]
#   --gui / --no-gui   install gtkwave + pulseview (default: yes, except in containers)
#   --no-vicoco        skip vicoco (Vivado-simulator hooks for cocotb via lab-bc)
#   --no-shell-rc      don't add the 'use6205' alias block to ~/.bashrc / ~/.zshrc
#   --upgrade          rebuild openFPGALoader, reinstall git-sourced Python packages
#   --skip-selftest    skip the cocotb + iverilog smoke test
#   -h, --help         show this help
#
# Environment overrides [defaults]:
#   VENV_DIR [~/6205_python]   PREFIX [/usr/local]    CACHE_DIR [~/.cache/6205-setup]
#   COCOTB_VERSION [1.9.2]     OFL_VERSION [v1.1.1]   IVERILOG_TAG [v12_0]
#
# Exit codes: 0 all good | 1 a required step failed |
#             2 finished, but an optional step failed | 130 interrupted
# =============================================================================

# --- Guard: must be bash >= 4.4 (checked before any bash-only syntax runs) ---
if [ -z "${BASH_VERSION:-}" ]; then
  echo "Please run with bash:  bash $0" >&2
  exit 1
fi
if (( BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 4) )); then
  echo "bash >= 4.4 required (found $BASH_VERSION)" >&2
  exit 1
fi

set -Eeuo pipefail
shopt -s inherit_errexit

# --- Configuration -----------------------------------------------------------
if [[ -z ${HOME:-} ]]; then
  HOME=$(getent passwd "$(id -un)" | cut -d: -f6)
  export HOME
fi
VENV_DIR="${VENV_DIR:-$HOME/6205_python}"
PREFIX="${PREFIX:-/usr/local}"
CACHE_DIR="${CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/6205-setup}"
COCOTB_VERSION="${COCOTB_VERSION:-1.9.2}"   # what the course docs pin
OFL_VERSION="${OFL_VERSION:-v1.1.1}"
IVERILOG_TAG="${IVERILOG_TAG:-v12_0}"      # only used if the distro's is too old
IVERILOG_MIN_MAJOR=12

readonly OFL_REPO="https://github.com/trabucayre/openFPGALoader.git"
readonly IVERILOG_REPO="https://github.com/steveicarus/iverilog.git"
readonly LAB_BC_SPEC="git+https://github.com/jodalyst/lab_bc_client"
readonly VICOCO_SPEC="git+https://github.com/kiran-vuksanaj/vicoco.git@stable"
readonly UDEV_RULES_DEST="/etc/udev/rules.d/99-openfpgaloader.rules"
readonly RC_MARK_BEGIN="# >>> 6205 toolchain >>>"
readonly RC_MARK_END="# <<< 6205 toolchain <<<"
readonly RC_UNCHANGED=100   # step exit code meaning "already correct, did nothing"

WANT_GUI=auto WANT_VICOCO=1 WANT_RC=1 UPGRADE=0 SELFTEST=1
PM="" IN_CONTAINER=0 IN_WSL=0 RUN_DIR="" LOG_FILE="" SUDO_KEEPALIVE_PID=""
OPTIONAL_FAILED=0
declare -a SUMMARY=()

# --- Output helpers ----------------------------------------------------------
if [[ -t 1 ]]; then
  C_B=$'\e[1m' C_G=$'\e[32m' C_Y=$'\e[33m' C_R=$'\e[31m' C_0=$'\e[0m'
else
  C_B="" C_G="" C_Y="" C_R="" C_0=""
fi
info() { printf '%s==> %s%s\n' "$C_B" "$*" "$C_0"; }
ok()   { printf '%s  [ok] %s%s\n' "$C_G" "$*" "$C_0"; }
warn() { printf '%s  [warn] %s%s\n' "$C_Y" "$*" "$C_0" >&2; }
err()  { printf '%s  [error] %s%s\n' "$C_R" "$*" "$C_0" >&2; }
die()  { err "$*"; exit 1; }
# Leave a note for the final summary (works from inside step subshells).
note() { printf '%s\n' "$*" >> "$RUN_DIR/notes"; }
# End the current step reporting "already correct". Only valid inside a step.
unchanged() { ok "$*"; exit "$RC_UNCHANGED"; }

# Report the failing command, except bare 'return N' from helpers that already
# printed a specific error message.
on_err() {
  [[ $3 == return* ]] && return 0
  err "command failed (exit $1) at line $2: $3"
}
trap 'on_err $? $LINENO "$BASH_COMMAND"' ERR

usage() { awk 'NR > 2 && /^# =====/ { exit } NR > 2 { sub(/^# ?/, ""); print }' "$0"; }

# --- Generic helpers ---------------------------------------------------------
as_root() {
  if (( EUID == 0 )); then "$@"; else sudo "$@"; fi
}

# retry <attempts> <command...>  (exponential backoff; for network-ish things)
retry() {
  local attempts=$1 n=1 delay=3
  shift
  while true; do
    if "$@"; then return 0; fi
    if (( n >= attempts )); then return 1; fi
    warn "attempt $n/$attempts failed: $* (retrying in ${delay}s)"
    sleep "$delay"
    n=$((n + 1)); delay=$((delay * 2))
  done
}

# quietly <label> <command...>: run a noisy command with output captured to a
# log file; on failure, show the tail and where the full log is.
quietly() {
  local label=$1 log
  shift
  log="$CACHE_DIR/logs/$label.log"
  local rc=0
  "$@" > "$log" 2>&1 || rc=$?
  if (( rc == 0 )); then return 0; fi
  err "$label failed (exit $rc); last lines of $log:"
  tail -n 25 "$log" | sed 's/^/      /' >&2
  return "$rc"
}

nproc_safe() { nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2; }

same_file() {
  [[ -f $1 && -f $2 ]] && [[ $(cksum < "$1") == "$(cksum < "$2")" ]]
}

# fetch_tag <repo> <tag> <dest>: leave a clean checkout of exactly <tag> in <dest>.
# Reuses an existing clone when possible; otherwise re-clones shallowly.
fetch_tag() {
  local repo=$1 tag=$2 dest=$3
  if [[ -d $dest/.git ]] \
     && [[ $(git -C "$dest" describe --tags --exact-match 2>/dev/null) == "$tag" ]] \
     && git -C "$dest" reset --quiet --hard 2>/dev/null \
     && git -C "$dest" clean -fdxq 2>/dev/null; then
    return 0
  fi
  if [[ -e $dest ]]; then
    rm -rf "$dest" 2>/dev/null || as_root rm -rf "$dest" || return 1
  fi
  mkdir -p "$(dirname "$dest")" || return 1
  # Tell "tag doesn't exist" (don't retry) apart from "network trouble" (do).
  local n rc git_err
  for n in 1 2 3; do
    rc=0
    git_err=$(git ls-remote --exit-code --tags "$repo" "refs/tags/$tag" 2>&1 >/dev/null) || rc=$?
    case $rc in
      0) break ;;
      2) err "tag '$tag' does not exist in $repo"; return 1 ;;
    esac
    if (( n == 3 )); then
      err "can't reach $repo. git said: ${git_err:-<no output>}"
      if [[ $git_err == *certificate* ]]; then
        warn "TLS verification failed: behind a TLS-inspecting proxy? Add its CA to the system trust store (e.g. /usr/local/share/ca-certificates + update-ca-certificates)."
      fi
      return 1
    fi
    warn "can't reach $repo (attempt $n/3); retrying in $((n * 3))s"
    sleep $((n * 3))
  done
  if ! retry 3 git clone --quiet --depth 1 --branch "$tag" \
         -c advice.detachedHead=false "$repo" "$dest"; then
    err "could not clone $repo at tag '$tag'"
    rm -rf "$dest" 2>/dev/null || true
    return 1
  fi
}

# --- Package manager abstraction ---------------------------------------------
# pkgs <group>: distro-specific package names for a logical group.
# dnf and zypper install by pkgconfig() capability, so library -devel package
# names don't have to be guessed per distro version. Verified: Fedora ships
# iverilog 13 and libftdi-devel/hidapi-devel; Arch ships iverilog, libftdi,
# hidapi, gtkwave, pulseview. openSUSE has no official iverilog (only the
# experimental "electronics" repo), so it falls through to a source build.
pkgs() {
  case "$PM:$1" in
    apt-get:base)   echo git ca-certificates build-essential cmake pkg-config gzip python3 python3-venv python3-dev python3-pip ;;
    apt-get:ofl)    echo libftdi1-dev zlib1g-dev ;;
    apt-get:oflopt) echo libhidapi-dev libudev-dev ;;
    apt-get:ivbld)  echo autoconf gperf flex bison ;;
    dnf:base)       echo git ca-certificates gcc gcc-c++ make cmake pkgconf-pkg-config gzip python3 python3-devel python3-pip ;;
    dnf:ofl)        echo 'pkgconfig(libftdi1)' 'pkgconfig(zlib)' ;;
    dnf:oflopt)     echo 'pkgconfig(hidapi-hidraw)' 'pkgconfig(libudev)' ;;
    dnf:ivbld)      echo autoconf gperf flex bison ;;
    pacman:base)    echo git ca-certificates base-devel cmake pkgconf gzip python python-pip ;;
    pacman:ofl)     echo libftdi zlib ;;
    pacman:oflopt)  echo hidapi systemd-libs ;;
    pacman:ivbld)   echo autoconf gperf flex bison ;;
    zypper:base)    echo git ca-certificates gcc gcc-c++ make cmake pkg-config gzip python3 python3-devel python3-pip ;;
    zypper:ofl)     echo 'pkgconfig(libftdi1)' 'pkgconfig(zlib)' ;;
    zypper:oflopt)  echo 'pkgconfig(hidapi-hidraw)' 'pkgconfig(libudev)' ;;
    zypper:ivbld)   echo autoconf gperf flex bison ;;
    *:iverilog)     echo iverilog ;;
    *:gtkwave)      echo gtkwave ;;
    *:pulseview)    echo pulseview ;;
    *) err "no package mapping for $PM:$1"; return 1 ;;
  esac
}

pkg_installed() {
  case $PM in
    apt-get)    [[ $(dpkg-query -W -f='${Status}' "$1" 2>/dev/null) == "install ok installed" ]] ;;
    dnf|zypper) rpm -q --whatprovides "$1" >/dev/null 2>&1 ;;
    pacman)     pacman -T "$1" >/dev/null 2>&1 ;;
  esac
}

all_installed() {
  local p
  for p in "$@"; do
    if ! pkg_installed "$p"; then return 1; fi
  done
}

# A refresh error is often one broken third-party repo while the distro repos
# updated fine, so warn and let the install itself be the real test.
pkg_refresh_once() {
  [[ -e $RUN_DIR/pkg-refreshed ]] && return 0
  touch "$RUN_DIR/pkg-refreshed"
  local refresh_ok=1
  case $PM in
    apt-get) retry 2 quietly pkg-refresh as_root apt-get -o DPkg::Lock::Timeout=300 update || refresh_ok=0 ;;
    zypper)  retry 2 quietly pkg-refresh as_root zypper --non-interactive --gpg-auto-import-keys refresh || refresh_ok=0 ;;
    *) : ;;  # dnf refreshes on its own; pacman syncs as part of -Syu below
  esac
  if (( ! refresh_ok )); then
    warn "package index refresh reported errors (often a broken third-party repo); trying the install anyway"
    note "Your package manager's index refresh had errors; check your repo config if installs fail."
  fi
  return 0
}

pkg_do_install() {
  case $PM in
    apt-get) as_root env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 \
               install -y -q --no-install-recommends "$@" ;;
    dnf)     as_root dnf install -y -q "$@" ;;
    pacman)  as_root pacman -Syu --needed --noconfirm "$@" ;;   # Arch: never partial-upgrade
    zypper)  as_root zypper --non-interactive install --no-recommends "$@" ;;
  esac
}

# ensure_pkgs <pkg...>: install whatever is missing. On a batch failure, retries
# one at a time so the error names the exact package(s) that can't be installed.
ensure_pkgs() {
  local p missing=() failed=()
  for p in "$@"; do
    if ! pkg_installed "$p"; then missing+=("$p"); fi
  done
  if (( ${#missing[@]} == 0 )); then return 0; fi
  info "  installing: ${missing[*]}"
  pkg_refresh_once
  if retry 2 quietly pkg-install pkg_do_install "${missing[@]}"; then return 0; fi
  warn "batch install failed; trying packages one by one to isolate the problem"
  for p in "${missing[@]}"; do
    if pkg_installed "$p"; then continue; fi
    if ! quietly "pkg-install-$p" pkg_do_install "$p"; then failed+=("$p"); fi
  done
  if (( ${#failed[@]} )); then
    err "could not install: ${failed[*]}"
    if [[ $PM == apt-get ]] && grep -qi '^ID=ubuntu' /etc/os-release 2>/dev/null; then
      warn "Ubuntu: several of these live in 'universe'. Try: sudo add-apt-repository -y universe (or add it to your apt sources)"
    fi
    if [[ $PM == dnf ]] && grep -qiE '^ID(_LIKE)?=.*(rhel|centos)' /etc/os-release 2>/dev/null; then
      warn "RHEL-family: these live in EPEL. Try: sudo dnf install -y epel-release && sudo dnf config-manager --set-enabled crb"
    fi
    return 1
  fi
}

# --- Version probes (print nothing if the tool is absent) --------------------
iverilog_major() {
  command -v iverilog >/dev/null 2>&1 || return 0
  { iverilog -V 2>&1 || true; } | sed -nE '1s/^Icarus Verilog version ([0-9]+).*/\1/p'
}
iverilog_line() {
  if ! command -v iverilog >/dev/null 2>&1; then echo "not found"; return 0; fi
  { iverilog -V 2>&1 || true; } | sed -n '1p'
}
ofl_version() {
  command -v openFPGALoader >/dev/null 2>&1 || return 0
  { openFPGALoader --Version 2>&1 || true; } | sed -nE 's/.*openFPGALoader (v[0-9][0-9.]*).*/\1/p' | sed -n '1p'
}
venv_ok() {
  [[ -x $VENV_DIR/bin/python ]] &&
    "$VENV_DIR/bin/python" -c 'import sys, pip; sys.exit(0 if sys.prefix != sys.base_prefix else 1)' \
      >/dev/null 2>&1
}
pip_q() { "$VENV_DIR/bin/python" -m pip --disable-pip-version-check --no-input -q "$@"; }
dist_version() {  # installed version of a Python distribution in the venv, or nothing
  "$VENV_DIR/bin/python" - "$1" <<'PY' 2>/dev/null || true
import sys
from importlib import metadata
name = sys.argv[1]
for n in (name, name.replace("_", "-"), name.replace("-", "_")):
    try:
        print(metadata.version(n)); break
    except metadata.PackageNotFoundError:
        pass
PY
}

# =============================================================================
# Steps. Each runs in its own subshell with errexit on. Exit 0 = changed
# something, RC_UNCHANGED = already correct, anything else = failed.
# =============================================================================

step_system_packages() {
  local req=() opt=()
  read -ra req <<< "$(pkgs base) $(pkgs ofl)"
  read -ra opt <<< "$(pkgs oflopt)"
  if all_installed "${req[@]}" "${opt[@]}"; then
    unchanged "build dependencies already present"
  fi
  ensure_pkgs "${req[@]}"
  # hidapi/libudev only enable extra cables in openFPGALoader; the Urbana's FTDI
  # works without them, so a failure here downgrades features instead of failing.
  if ! ensure_pkgs "${opt[@]}"; then
    note "Optional openFPGALoader deps (${opt[*]}) unavailable; building without CMSIS-DAP/udev scan. The Urbana board (FTDI) is unaffected."
  fi
  ok "build dependencies installed"
}

step_iverilog() {
  local have
  have=$(iverilog_major)
  if [[ -n $have ]] && (( have >= IVERILOG_MIN_MAJOR )); then
    unchanged "$(iverilog_line)"
  fi
  if [[ -n $have ]]; then warn "found iverilog $have.x, need >= $IVERILOG_MIN_MAJOR"; fi

  info "  trying the distro package first"
  if ensure_pkgs "$(pkgs iverilog)"; then
    hash -r
    have=$(iverilog_major)
    if [[ -n $have ]] && (( have >= IVERILOG_MIN_MAJOR )); then
      ok "$(iverilog_line)"
      return 0
    fi
    warn "distro iverilog is ${have:-unknown}.x; building $IVERILOG_TAG from source instead"
  else
    warn "no usable distro package; building $IVERILOG_TAG from source"
  fi

  local ivb=()
  read -ra ivb <<< "$(pkgs ivbld)"
  ensure_pkgs "${ivb[@]}"
  local src="$CACHE_DIR/src/iverilog-$IVERILOG_TAG"
  fetch_tag "$IVERILOG_REPO" "$IVERILOG_TAG" "$src"
  info "  building iverilog (a few minutes)"
  quietly iverilog-autoconf bash -c 'cd "$1" && sh autoconf.sh' _ "$src"
  quietly iverilog-configure bash -c 'cd "$1" && ./configure --prefix="$2"' _ "$src" "$PREFIX"
  quietly iverilog-make make -C "$src" -j"$(nproc_safe)"
  quietly iverilog-install as_root make -C "$src" install
  hash -r
  have=$(iverilog_major)
  if [[ -z $have ]] || (( have < IVERILOG_MIN_MAJOR )); then
    err "built iverilog into $PREFIX/bin, but '$(command -v iverilog || echo none)' is first on PATH"
    return 1
  fi
  ok "$(iverilog_line) (from source, in $PREFIX)"
}

step_openfpgaloader() {
  local have
  have=$(ofl_version)
  if [[ $have == "$OFL_VERSION" ]] && (( ! UPGRADE )); then
    unchanged "openFPGALoader $have"
  fi
  if [[ -n $have ]]; then info "  found openFPGALoader $have, want $OFL_VERSION"; fi

  local src="$CACHE_DIR/src/openFPGALoader-$OFL_VERSION"
  local build="$CACHE_DIR/build/openFPGALoader-$OFL_VERSION"
  local flags=(-DCMAKE_BUILD_TYPE=Release "-DCMAKE_INSTALL_PREFIX=$PREFIX")
  if ! pkg-config --exists hidapi-hidraw 2>/dev/null && ! pkg-config --exists hidapi-libusb 2>/dev/null; then
    flags+=(-DENABLE_CMSISDAP=OFF)
  fi
  if ! pkg-config --exists libudev 2>/dev/null; then flags+=(-DENABLE_UDEV=OFF); fi

  fetch_tag "$OFL_REPO" "$OFL_VERSION" "$src"
  rm -rf "$build" 2>/dev/null || as_root rm -rf "$build"
  info "  building openFPGALoader (a minute or two)"
  quietly ofl-configure cmake -S "$src" -B "$build" "${flags[@]}"
  quietly ofl-build cmake --build "$build" -j "$(nproc_safe)"
  quietly ofl-install as_root cmake --build "$build" --target install
  hash -r
  have=$(ofl_version)
  if [[ $have != "$OFL_VERSION" ]]; then
    err "installed $OFL_VERSION to $PREFIX/bin, but PATH resolves '$(command -v openFPGALoader || echo none)' (${have:-no version})"
    return 1
  fi
  ok "openFPGALoader $have installed to $PREFIX"
}

step_udev() {
  local changed=0 user
  local src="$CACHE_DIR/src/openFPGALoader-$OFL_VERSION"
  if [[ ! -f $src/99-openfpgaloader.rules ]]; then
    fetch_tag "$OFL_REPO" "$OFL_VERSION" "$src"
  fi

  if ! same_file "$src/99-openfpgaloader.rules" "$UDEV_RULES_DEST"; then
    as_root install -D -m 0644 "$src/99-openfpgaloader.rules" "$UDEV_RULES_DEST"
    ok "installed $UDEV_RULES_DEST"
    changed=1
  fi

  if ! getent group plugdev >/dev/null; then
    as_root groupadd --system plugdev
    ok "created group plugdev"
    changed=1
  fi

  user=$(id -un)
  if [[ $user != root && " $(id -nG "$user") " != *" plugdev "* ]]; then
    as_root usermod -aG plugdev "$user"
    ok "added $user to plugdev"
    note "Log out and back in (or reboot the VM) so your plugdev membership takes effect."
    changed=1
  fi

  if (( changed )); then
    if (( IN_CONTAINER )); then
      : # covered by the container note in the summary
    elif command -v udevadm >/dev/null 2>&1 && [[ -d /run/udev ]]; then
      if as_root udevadm control --reload-rules && as_root udevadm trigger; then
        ok "udev rules reloaded"
      else
        note "udev reload failed; unplug/replug the board (or reboot) to apply the rules."
      fi
    else
      note "udev isn't running here; the rules apply once it is (e.g. after a reboot)."
    fi
    return 0
  fi
  unchanged "udev rule and plugdev membership already in place"
}

step_python_env() {
  local py changed=0
  py=$(command -v python3) || { err "python3 not found"; return 1; }
  if ! "$py" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 8) else 1)'; then
    err "Python >= 3.8 required (found $("$py" -V 2>&1))"
    return 1
  fi
  if "$py" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 14) else 1)'; then
    warn "$("$py" -V 2>&1) is newer than cocotb $COCOTB_VERSION targets; if its install fails, try COCOTB_VERSION=... or an older python3"
  fi

  # A half-created or stale venv (e.g. its Python was upgraded away) is moved
  # aside rather than deleted, in case it holds anything you care about.
  if [[ -e $VENV_DIR ]] && ! venv_ok; then
    local bak
    bak="$VENV_DIR.broken-$(date +%Y%m%d-%H%M%S)"
    warn "$VENV_DIR exists but isn't a working venv; moving it to $bak"
    mv "$VENV_DIR" "$bak"
  fi
  if [[ ! -e $VENV_DIR ]]; then
    if ! "$py" -m venv "$VENV_DIR"; then
      rm -rf "$VENV_DIR"
      err "venv creation failed (on Debian/Ubuntu this needs the python3-venv package)"
      return 1
    fi
    retry 2 pip_q install --upgrade pip || warn "couldn't upgrade pip; continuing with the bundled one"
    ok "created venv $VENV_DIR"
    changed=1
  fi

  local reqs=()
  [[ $(dist_version cocotb) == "$COCOTB_VERSION" ]] || reqs+=("cocotb==$COCOTB_VERSION")
  [[ -n $(dist_version pyserial) ]] || reqs+=(pyserial)
  if (( ${#reqs[@]} )); then
    retry 2 pip_q install "${reqs[@]}"
    ok "installed ${reqs[*]}"
    changed=1
  fi
  if [[ -z $(dist_version lab_bc_client) ]] || (( UPGRADE )); then
    retry 2 pip_q install --upgrade "$LAB_BC_SPEC"
    ok "installed lab-bc client"
    changed=1
  fi

  # Verify rather than trust pip's exit code.
  "$VENV_DIR/bin/python" -c 'import cocotb, serial' \
    || { err "cocotb/pyserial not importable in $VENV_DIR"; return 1; }
  [[ -x $VENV_DIR/bin/lab-bc ]] || { err "lab-bc entry point missing from $VENV_DIR/bin"; return 1; }

  if (( changed )); then return 0; fi
  unchanged "venv has cocotb $COCOTB_VERSION, pyserial, lab-bc"
}

step_vicoco() {
  if [[ -n $(dist_version vicoco) ]] && (( ! UPGRADE )); then
    unchanged "vicoco $(dist_version vicoco)"
  fi
  retry 2 pip_q install --upgrade "$VICOCO_SPEC"
  ok "installed vicoco"
}

step_gtkwave() {
  if all_installed "$(pkgs gtkwave)"; then unchanged "gtkwave already installed"; fi
  ensure_pkgs "$(pkgs gtkwave)"
  ok "gtkwave installed"
}

step_pulseview() {
  if all_installed "$(pkgs pulseview)"; then unchanged "pulseview already installed"; fi
  ensure_pkgs "$(pkgs pulseview)"
  ok "pulseview installed"
}

render_rc_block() {
  printf '# Added by install_6205.sh; safe to delete this whole block.\n'
  printf "alias use6205='source \"%s/bin/activate\"'\n" "$VENV_DIR"
  if [[ ":$PATH:" != *":$PREFIX/bin:"* ]]; then
    printf 'export PATH="%s/bin:$PATH"\n' "$PREFIX"
  fi
}

# upsert_block <file> <content>: prints "changed" or "same".
upsert_block() {
  local file content=$2 current tmp
  file=$(readlink -f "$1")   # write through dotfile symlinks instead of replacing them
  touch "$file"
  if grep -qxF "$RC_MARK_BEGIN" "$file" && ! grep -qxF "$RC_MARK_END" "$file"; then
    err "$file has '$RC_MARK_BEGIN' but no end marker; fix it by hand, not touching it"
    return 1
  fi
  current=$(awk -v b="$RC_MARK_BEGIN" -v e="$RC_MARK_END" '$0==b{f=1;next} $0==e{f=0;next} f' "$file")
  if grep -qxF "$RC_MARK_BEGIN" "$file" && [[ $current == "$content" ]]; then
    echo same
    return 0
  fi
  tmp=$(mktemp "$file.XXXXXX")
  awk -v b="$RC_MARK_BEGIN" -v e="$RC_MARK_END" '$0==b{s=1;next} $0==e{s=0;next} !s' "$file" > "$tmp"
  printf '%s\n%s\n%s\n' "$RC_MARK_BEGIN" "$content" "$RC_MARK_END" >> "$tmp"
  chmod --reference="$file" "$tmp" 2>/dev/null || true
  mv "$tmp" "$file"
  echo changed
}

step_shell_rc() {
  local block rc result changed=0
  block=$(render_rc_block)
  for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    # Always manage .bashrc; only touch .zshrc if you already have one.
    if [[ $rc == */.zshrc && ! -e $rc ]]; then continue; fi
    result=$(upsert_block "$rc" "$block")
    if [[ $result == changed ]]; then ok "updated $rc"; changed=1; fi
  done
  if (( changed )); then return 0; fi
  unchanged "shell alias 'use6205' already configured"
}

step_selftest() {
  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  cat > "$tmp/counter.sv" <<'SV'
module counter (input wire clk, input wire rst, output logic [7:0] count);
  always_ff @(posedge clk) begin
    if (rst) count <= 8'd0;
    else     count <= count + 8'd1;
  end
endmodule
SV
  cat > "$tmp/test_counter.py" <<'PY'
import cocotb
from cocotb.clock import Clock
from cocotb.triggers import ClockCycles, ReadOnly, RisingEdge

@cocotb.test()
async def counts_up(dut):
    cocotb.start_soon(Clock(dut.clk, 10, units="ns").start())
    dut.rst.value = 1
    await ClockCycles(dut.clk, 2)
    dut.rst.value = 0
    await RisingEdge(dut.clk)
    await ReadOnly()
    a = int(dut.count.value)
    await RisingEdge(dut.clk)
    await ReadOnly()
    b = int(dut.count.value)
    assert b == (a + 1) % 256, f"count went {a} -> {b}"
PY
  cat > "$tmp/run.py" <<'PY'
import sys
from pathlib import Path
try:
    from cocotb.runner import get_runner, get_results          # cocotb 1.x
except ImportError:
    from cocotb_tools.runner import get_runner, get_results    # cocotb 2.x
here = Path(__file__).resolve().parent
r = get_runner("icarus")
r.build(sources=[here / "counter.sv"], hdl_toplevel="counter",
        build_dir=here / "sim_build", always=True, timescale=("1ns", "1ps"))
xml = r.test(hdl_toplevel="counter", test_module="test_counter",
             build_dir=here / "sim_build", test_dir=here)
total, failed = get_results(xml)
sys.exit(1 if failed or not total else 0)
PY
  if ! (cd "$tmp" && "$VENV_DIR/bin/python" run.py > "$tmp/out.log" 2>&1); then
    cat "$tmp/out.log" >&2
    err "cocotb + iverilog smoke test failed (output above)"
    return 1
  fi
  unchanged "cocotb + iverilog smoke test passed"
}

# =============================================================================
# Orchestration
# =============================================================================

# run_step <critical|optional> <description> <function>
run_step() {
  local kind=$1 desc=$2 fn=$3 rc
  info "$desc"
  trap - ERR
  set +e
  (
    trap 'on_err $? $LINENO "$BASH_COMMAND"' ERR
    set -Eeuo pipefail
    "$fn"
  )
  rc=$?
  set -e
  trap 'on_err $? $LINENO "$BASH_COMMAND"' ERR
  case $rc in
    0)               SUMMARY+=("CHANGED  $desc") ;;
    "$RC_UNCHANGED") SUMMARY+=("ok       $desc") ;;
    130)             exit 130 ;;
    *)
      if [[ $kind == critical ]]; then
        SUMMARY+=("FAILED   $desc")
        print_summary
        die "A required step failed (see the error above). Fix it and re-run; finished steps will be skipped."
      fi
      SUMMARY+=("FAILED   $desc (optional, continued)")
      OPTIONAL_FAILED=1
      warn "optional step failed; continuing"
      ;;
  esac
}

print_summary() {
  local line
  printf '\n%s============================ Summary ============================%s\n' "$C_B" "$C_0"
  for line in "${SUMMARY[@]}"; do printf '  %s\n' "$line"; done
  local ofl
  ofl=$(ofl_version)
  printf '\n  iverilog:        %s\n' "$(iverilog_line)"
  printf '  openFPGALoader:  %s\n' "${ofl:-not found}"
  if venv_ok; then
    printf '  cocotb:          %s   (venv: %s)\n' "$(dist_version cocotb)" "$VENV_DIR"
  fi
  if [[ -s $RUN_DIR/notes ]]; then
    printf '\n%sNotes:%s\n' "$C_Y" "$C_0"
    sort -u "$RUN_DIR/notes" | sed 's/^/  - /'
  fi
  printf '\n  Log: %s\n' "$LOG_FILE"
}

print_next_steps() {
  cat <<EOF

${C_B}Next steps${C_0}
  1. Open a new shell (or 'source ~/.bashrc'), then run:  use6205
     (same as: source $VENV_DIR/bin/activate)
  2. One-time lab-bc setup:  lab-bc configure
     (kerberos, MIT ID, and the server endpoint listed on the course's Vivado page)
  3. Flashing/UART need the board's USB device inside this machine:
       VM:        add a USB passthrough filter for 0403:6010 (FTDI FT2232H)
       WSL2:      usbipd bind / usbipd attach --wsl --hardware-id=0403:6010
       Docker:    docker run --device=/dev/bus/usb ... (Linux hosts only)
     Then check with:  openFPGALoader --detect
EOF
}

preflight() {
  [[ $(uname -s) == Linux ]] || die "This script targets Linux (run it inside your VM/container)."

  if (( EUID == 0 )) && [[ -n ${SUDO_USER:-} && $SUDO_USER != root ]]; then
    die "Don't run this with sudo. Run it as '$SUDO_USER'; it asks for sudo only when needed, so your venv stays owned by you."
  fi

  local pm
  for pm in apt-get dnf pacman zypper; do
    if command -v "$pm" >/dev/null 2>&1; then PM=$pm; break; fi
  done
  [[ -n $PM ]] || die "No supported package manager (apt-get/dnf/pacman/zypper). Install git, cmake, a C++ compiler, libftdi1 dev headers, python3 + venv, and iverilog >= 12 manually, then re-run."

  if [[ -f /.dockerenv || -f /run/.containerenv ]] \
     || grep -qaE 'docker|lxc|kubepods|containerd' /proc/1/cgroup 2>/dev/null; then
    IN_CONTAINER=1
  fi
  if command -v systemd-detect-virt >/dev/null 2>&1 && systemd-detect-virt -cq 2>/dev/null; then
    IN_CONTAINER=1
  fi
  if grep -qi microsoft /proc/sys/kernel/osrelease 2>/dev/null; then IN_WSL=1; fi

  if [[ $WANT_GUI == auto ]]; then
    if (( IN_CONTAINER )); then WANT_GUI=0; else WANT_GUI=1; fi
  fi

  mkdir -p "$CACHE_DIR/src" "$CACHE_DIR/build"
  if command -v flock >/dev/null 2>&1; then
    exec 9> "$CACHE_DIR/.lock"
    flock -n 9 || die "Another install_6205.sh is already running."
  fi

  local avail_kb
  avail_kb=$(df -Pk "$CACHE_DIR" 2>/dev/null | awk 'NR==2 {print $4}') || avail_kb=""
  if [[ -n $avail_kb ]] && (( avail_kb < 2000000 )); then
    warn "less than 2 GB free under $CACHE_DIR; builds may run out of space"
  fi

  if (( EUID != 0 )); then
    command -v sudo >/dev/null 2>&1 || die "Not root and 'sudo' isn't installed. Re-run as root or install sudo."
    info "Requesting sudo (used only for packages, $PREFIX, and udev)"
    sudo -v || die "sudo authentication failed (non-interactive? configure NOPASSWD or run as root)."
    # fd 9 (the lock) is closed so a lingering 'sleep' can't keep the lock held.
    ( exec 9>&-; while kill -0 "$$" 2>/dev/null; do sudo -n true 2>/dev/null || true; sleep 50; done ) &
    SUDO_KEEPALIVE_PID=$!
  fi

  info "Environment: $PM, $(uname -m)$( (( IN_CONTAINER )) && echo ', container')$( (( IN_WSL )) && echo ', WSL')"
}

cleanup() {
  local rc=$?
  if [[ -n $SUDO_KEEPALIVE_PID ]]; then kill "$SUDO_KEEPALIVE_PID" 2>/dev/null || true; fi
  if [[ -n $RUN_DIR ]]; then rm -rf "$RUN_DIR"; fi
  exit "$rc"
}

main() {
  while (( $# )); do
    case $1 in
      --gui)           WANT_GUI=1 ;;
      --no-gui)        WANT_GUI=0 ;;
      --no-vicoco)     WANT_VICOCO=0 ;;
      --no-shell-rc)   WANT_RC=0 ;;
      --upgrade)       UPGRADE=1 ;;
      --skip-selftest) SELFTEST=0 ;;
      -h|--help)       usage; exit 0 ;;
      *)               echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
    shift
  done

  mkdir -p "$CACHE_DIR/logs" || { echo "cannot create $CACHE_DIR" >&2; exit 1; }
  LOG_FILE="$CACHE_DIR/logs/install-$(date +%Y%m%d-%H%M%S).log"
  exec > >(tee -a "$LOG_FILE") 2>&1
  RUN_DIR=$(mktemp -d)
  trap cleanup EXIT
  trap 'err "Interrupted. Re-run any time; it picks up where it left off."; exit 130' INT TERM

  preflight

  run_step critical "System build dependencies"          step_system_packages
  run_step critical "Icarus Verilog >= $IVERILOG_MIN_MAJOR" step_iverilog
  run_step critical "openFPGALoader $OFL_VERSION"        step_openfpgaloader
  run_step optional "USB access (udev rule, plugdev)"    step_udev
  run_step critical "Python venv: cocotb $COCOTB_VERSION, pyserial, lab-bc" step_python_env

  if (( WANT_VICOCO )); then run_step optional "vicoco" step_vicoco
  else SUMMARY+=("skipped  vicoco (--no-vicoco)"); fi

  if (( WANT_GUI )); then
    run_step optional "gtkwave"   step_gtkwave
    run_step optional "pulseview" step_pulseview
  else
    SUMMARY+=("skipped  gtkwave, pulseview (headless; pass --gui to install)")
  fi

  if (( WANT_RC )); then run_step optional "Shell alias 'use6205'" step_shell_rc
  else SUMMARY+=("skipped  shell alias (--no-shell-rc)"); fi

  if (( SELFTEST )); then run_step optional "Smoke test (cocotb + iverilog)" step_selftest
  else SUMMARY+=("skipped  smoke test (--skip-selftest)"); fi

  if (( IN_CONTAINER )); then
    note "Container: udev rules only matter on the HOST. Install $UDEV_RULES_DEST (from openFPGALoader) there, and pass the board in with --device."
  fi
  if (( IN_WSL )) && [[ ! -d /run/systemd/system ]]; then
    note "WSL: systemd isn't running. Add '[boot]' / 'systemd=true' to /etc/wsl.conf and run 'wsl --shutdown' so udev can apply the rules."
  fi

  print_summary
  print_next_steps
  if (( OPTIONAL_FAILED )); then
    warn "Finished, but some optional steps failed (see summary). Re-run after fixing to complete them."
    exit 2
  fi
}

main "$@"
