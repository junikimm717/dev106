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

WORKDIR /workspace


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
  bc \
  python3 && \
  rm -rf /var/lib/apt/lists/*

RUN echo "%dev106 ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev106 \
 && chmod 0440 /etc/sudoers.d/dev106

RUN ln -sfn python3 /usr/bin/python

# Fail the build if the course-required tools are missing or too old.
RUN qemu-system-riscv64 --version && \
  riscv64-linux-gnu-gcc --version && \
  gdb-multiarch --version

COPY --from=builder /workspace/bootstrap /bootstrap

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

WORKDIR /workspace
