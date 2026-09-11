FROM golang:trixie AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build ./cmd/bootstrap


##############################
# Base mit_6106 image
##############################

FROM debian:trixie-slim AS mit_6106

ARG STUDENT_REV

ARG TEMURIN8_URL="https://api.adoptium.net/v3/binary/latest/8/ga/linux/x64/jdk/hotspot/normal/eclipse"

RUN apt-get update && \
  apt-get install -y --no-install-recommends \
  sudo git tar wget make perl ssh unzip xz-utils curl ca-certificates \
  libxext-dev libx11-dev clang linux-perf valgrind \
  python3 \
  python3-dev \
  python3-pip \
  python3-setuptools \
  python3-wheel \
  tcl && \
  rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/java/openjdk8 && \
  curl -fsSL "$TEMURIN8_URL" -o /tmp/openjdk8.tar.gz && \
  tar -xzf /tmp/openjdk8.tar.gz --strip-components=1 -C /opt/java/openjdk8 && \
  rm /tmp/openjdk8.tar.gz

ENV JAVA_HOME=/opt/java/openjdk8
ENV PATH="${JAVA_HOME}/bin:${PATH}"

RUN echo "%dev106 ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev106 \
 && chmod 0440 /etc/sudoers.d/dev106

RUN ln -sfn python3 /usr/bin/python

RUN mkdir /student_software && \
  curl -L https://github.com/6106-spring26/student_software/archive/${STUDENT_REV}.tar.gz \
  | tar -xz --strip-components=1 -C /student_software && \
  cd /student_software && ./install.sh

RUN pip3 install --break-system-packages SQLAlchemy==1.4.54 opentuner

COPY --from=builder /workspace/bootstrap /bootstrap

# The repo root is bind-mounted here, so every image derived from this one
# should drop the user into it rather than /.
WORKDIR /workspace

ENTRYPOINT ["/bootstrap"]


##############################
# Final nvim image (6.106)
##############################

FROM mit_6106 AS nvim

RUN mkdir /nvim && \
  curl -L https://github.com/junikimm717/nvim2025/archive/master.tar.gz \
  | tar -xz --strip-components=1 -C /nvim

RUN <<EOF cat > /nvim/lua/configs/init.lua
---@type Config
return {
  mason = {},
  lazy = require("themes.kanagawa"),
  treesitter = {
    "json",
    "markdown",
    "gitignore",
    "gitcommit",
    "c",
    "cpp",
    "make",
    "python",
    "bash",
    "asm",
  }
}
EOF

RUN cd /nvim && make -j$(nproc)
RUN /nvim/build/bin/setup-nvim

COPY ./home /home/dev106

ENV DEV_CHOWN="/nvim"
ENV DEV_CHOWNEXCLUDE="/nvim/build/pkgs"


##############################
# Base mit_6181 image
# RISC-V toolchain for 6.1810 / xv6
# https://pdos.csail.mit.edu/6.1810/2026/tools.html
##############################

FROM debian:trixie-slim AS mit_6181

# Course packages (Ubuntu 24 names) plus qemu-system-riscv (Debian
# Trixie split RISC-V out of qemu-system-misc) and bc/perl (xv6 Makefile).
RUN apt-get update && \
  apt-get install -y --no-install-recommends \
  sudo git tar wget make perl ssh unzip xz-utils curl ca-certificates \
  build-essential \
  gdb-multiarch \
  qemu-system-misc \
  qemu-system-riscv \
  gcc-riscv64-linux-gnu \
  binutils-riscv64-linux-gnu \
  clangd \
  bc \
  python3 && \
  rm -rf /var/lib/apt/lists/*

RUN echo "%dev106 ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev106 \
 && chmod 0440 /etc/sudoers.d/dev106

RUN ln -sfn python3 /usr/bin/python

# `make qemu-gdb` prints "Now run 'gdb' in another window", but plain gdb is
# native-only and cannot set a riscv:rv64 target; it is here as a dependency
# of gdb-multiarch, which can. /usr/local/bin wins on PATH, and gdb-multiarch
# still autodetects the native architecture, so nothing is lost.
RUN ln -sf /usr/bin/gdb-multiarch /usr/local/bin/gdb

# The same target writes a .gdbinit at the repo root carrying the per-user
# port and `target remote`. gdb declines to auto-load it, leaving you in a
# gdb attached to nothing, so trust the one directory the repo is mounted at.
RUN printf 'add-auto-load-safe-path /workspace\n' >> /etc/gdb/gdbinit

# Fail the build if the course-required tools are missing or too old.
RUN qemu-system-riscv64 --version && \
  riscv64-linux-gnu-gcc --version && \
  gdb-multiarch --version && \
  clangd --version

# The two fixes above are the whole xv6 debugging path, so assert them rather
# than trusting that a package upgrade left them alone.
RUN gdb -batch -ex 'set architecture riscv:rv64' && \
  mkdir -p /workspace && \
  printf 'set architecture riscv:rv64\n' > /workspace/.gdbinit && \
  cd /workspace && \
  gdb -batch -ex 'show architecture' 2>&1 | tee /tmp/gdbcheck && \
  ! grep -qi declined /tmp/gdbcheck && \
  grep -q 'riscv:rv64' /tmp/gdbcheck && \
  rm /workspace/.gdbinit /tmp/gdbcheck

# Bake xv6 Makefile CFLAGS + RISC-V gcc builtins into clangd's user config
# so any image based on mit_6181 (nvim, VS Code, etc.) gets the same LSP.
# -I/workspace is the Makefile's -I. (repo root is bind-mounted there).
# gcc -print-file-name=include is the Linux stand-in for a cross-gcc
# builtin include; -nostdinc keeps host libc out.
RUN <<'EOF'
set -eu
GCC_INC="$(riscv64-linux-gnu-gcc -print-file-name=include)"
test -f "$GCC_INC/stdarg.h"
mkdir -p /home/dev106/.config/clangd
cat > /home/dev106/.config/clangd/config.yaml << YAML
CompileFlags:
  Add:
    - --target=riscv64-linux-gnu
    - -march=rv64gc
    - -mabi=lp64d
    - -Wall
    - -Werror
    - -Wno-unknown-attributes
    - -O
    - -fno-omit-frame-pointer
    - -ggdb
    - -gdwarf-2
    - -mcmodel=medany
    - -ffreestanding
    - -fno-common
    - -nostdlib
    - -fno-builtin-strncpy
    - -fno-builtin-strncmp
    - -fno-builtin-strlen
    - -fno-builtin-memset
    - -fno-builtin-memmove
    - -fno-builtin-memcmp
    - -fno-builtin-log
    - -fno-builtin-bzero
    - -fno-builtin-strchr
    - -fno-builtin-exit
    - -fno-builtin-malloc
    - -fno-builtin-putc
    - -fno-builtin-free
    - -fno-builtin-memcpy
    - -Wno-main
    - -fno-builtin-printf
    - -fno-builtin-fprintf
    - -fno-builtin-vprintf
    - -I/workspace
    - -I${GCC_INC}
    - -nostdinc
    - -fno-stack-protector
    - -fno-pie
YAML
EOF

COPY --from=builder /workspace/bootstrap /bootstrap

WORKDIR /workspace

ENTRYPOINT ["/bootstrap"]


##############################
# nvim image (6.1810)
##############################

FROM mit_6181 AS nvim_6181

RUN mkdir /nvim && \
  curl -L https://github.com/junikimm717/nvim2025/archive/master.tar.gz \
  | tar -xz --strip-components=1 -C /nvim

RUN <<EOF cat > /nvim/lua/configs/init.lua
---@type Config
return {
  mason = {},
  lazy = require("themes.kanagawa"),
  treesitter = {
    "json",
    "markdown",
    "gitignore",
    "gitcommit",
    "c",
    "cpp",
    "make",
    "python",
    "bash",
    "asm",
  }
}
EOF

RUN cd /nvim && make -j$(nproc)
RUN /nvim/build/bin/setup-nvim

COPY ./home /home/dev106

ENV DEV_CHOWN="/nvim"
ENV DEV_CHOWNEXCLUDE="/nvim/build/pkgs"


##############################
# Base mit_6205 image
# Open-source FPGA toolchain for 6.2050. Vivado is NOT installed; builds go
# to the course's remote server through lab-bc.
# https://fpga.mit.edu/6205/F26/documentation/vivado
##############################

FROM debian:trixie-slim AS mit_6205

# verilator is not part of the course toolchain, but it is the only thing
# here that elaborates the design: width mismatches, undriven and unused
# signals, inferred latches. Icarus catches undeclared identifiers, verible
# catches style. Neither catches a 8-bit value quietly truncated to 4.
RUN apt-get update && \
  apt-get install -y --no-install-recommends \
  sudo git tar wget make perl ssh unzip xz-utils curl ca-certificates \
  verilator \
  python3 && \
  rm -rf /var/lib/apt/lists/*

RUN echo "%dev106 ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev106 \
 && chmod 0440 /etc/sudoers.d/dev106

RUN ln -sfn python3 /usr/bin/python

# The venv goes outside /home/dev106 because bootstrap chowns the home
# directory on every start, and walking a venv full of pip packages each time
# is slow. /opt is stable and read-only to the student.
ENV VENV_6205=/opt/6205_python

COPY docker/install_6205.sh /tmp/install_6205.sh

# --no-gui: no X server here. --no-shell-rc: the alias is written below into
# the student's profile instead of root's. --skip-selftest: the smoke test
# wants a writable HOME, which does not exist until bootstrap runs.
#
# COCOTB_VERSION: the course docs say plain `pip3 install cocotb`, so a
# student following them gets 2.x, where the runner moved to
# cocotb_tools.runner. The installer defaults to 1.9.2, which leaves
# `from cocotb_tools.runner import get_runner` failing. Pinned rather than
# left floating so the image is reproducible.
#
# --no-vicoco: course staff say vicoco is not needed for now, and it cannot
# be installed as documented anyway -- its @stable pins cocotb==1.9.2, which
# cannot coexist with the above. `lab-bc simulate` does not need a local copy
# either: the server runs its own vicoco against its own cocotb. Nothing here
# should install it until the course asks for it.
RUN HOME=/root VENV_DIR="$VENV_6205" COCOTB_VERSION=2.1.0 \
  /tmp/install_6205.sh --no-gui --no-shell-rc --skip-selftest --no-vicoco && \
  rm /tmp/install_6205.sh

# The udev rule is useless inside the container (the kernel is the host's),
# but dev106 --group-adds the host GID that owns the device node, so the
# numeric group is what actually grants access.
RUN groupadd --system plugdev 2>/dev/null || true

# Activate the toolchain venv for every login shell, so `lab-bc`, `cocotb`,
# and `openFPGALoader` are simply on PATH.
RUN printf '%s\n' \
  '# 6.205 toolchain' \
  '. /opt/6205_python/bin/activate' \
  > /etc/profile.d/6205.sh

ENV PATH="/opt/6205_python/bin:${PATH}"

# Editors resolve the interpreter from VIRTUAL_ENV, not from PATH. Neovim's
# launcher prepends its own bundled python, so without this pyright reports
# every cocotb import as unresolved.
ENV VIRTUAL_ENV=/opt/6205_python

COPY --from=builder /workspace/bootstrap /bootstrap

WORKDIR /workspace

ENTRYPOINT ["/bootstrap"]


##############################
# nvim image (6.2050)
##############################

FROM mit_6205 AS nvim_6205

RUN mkdir /nvim && \
  curl -L https://github.com/junikimm717/nvim2025/archive/master.tar.gz \
  | tar -xz --strip-components=1 -C /nvim

RUN <<EOF cat > /nvim/lua/configs/init.lua
---@type Config
return {
  mason = {
    -- Style and syntax. Catches blocking assignments in always_ff and case
    -- statements with no default; cannot see undeclared signals or width
    -- mismatches, since it parses rather than elaborates.
    "verible",
    -- Indexes the project for cross-module definitions, and shells out to
    -- verilator for diagnostics, which is what actually catches the
    -- expensive mistakes.
    "svlangserver",
    -- cocotb testbenches. pyright, not basedpyright: cocotb's dut handle is
    -- dynamically typed, and strict mode buries real errors in noise.
    "pyright",
    "ruff",
  },
  lazy = require("themes.kanagawa"),
  treesitter = {
    "json",
    "markdown",
    "gitignore",
    "gitcommit",
    "c",
    "cpp",
    "make",
    "python",
    "bash",
    -- The parser is named systemverilog and covers .v too; "verilog" is
    -- silently skipped as an unknown language.
    "systemverilog",
    "tcl",
  }
}
EOF

RUN cd /nvim && make -j$(nproc)
RUN /nvim/build/bin/setup-nvim

# The launcher prepends neovim's bundled python, which shadows the 6.205 venv:
# pyright then calls every cocotb import unresolved, and :terminal gets the
# wrong interpreter. Runs after setup-nvim so the build is unaffected.
RUN sed -i '/^exec /i export PATH="/opt/6205_python/bin:$PATH"' /nvim/build/bin/nvim

COPY ./home /home/dev106

ENV DEV_CHOWN="/nvim"
ENV DEV_CHOWNEXCLUDE="/nvim/build/pkgs"
