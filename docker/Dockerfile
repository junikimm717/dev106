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

# Fail the build if the course-required tools are missing or too old.
RUN qemu-system-riscv64 --version && \
  riscv64-linux-gnu-gcc --version && \
  gdb-multiarch --version && \
  clangd --version

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
