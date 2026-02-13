FROM golang:trixie AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build ./cmd/bootstrap

FROM debian:trixie-slim AS mit_6106

RUN apt-get update && \
  apt-get install -y sudo git tar wget make perl ssh unzip xz-utils curl\
  libxext-dev libx11-dev clang linux-perf

RUN echo "%dev106 ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev106 \
 && chmod 0440 /etc/sudoers.d/dev106
RUN ln -sfn python3 /usr/bin/python

RUN mkdir /student_software && \
  curl -L https://github.com/6106-spring26/student_software/archive/96b68493ee65c11fdc3ca2564eebdc725b4dfb86.tar.gz \
  | tar -xz --strip-components=1 -C /student_software && \
  cd /student_software && ./install.sh

COPY --from=builder /workspace/bootstrap /bootstrap

ENTRYPOINT ["/bootstrap"]

FROM mit_6106

RUN mkdir /nvim && \
  curl -L https://github.com/junikimm717/nvim2025/archive/master.tar.gz\
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

# copy over some dotfiles
COPY ./home /home/dev106

ENV DEV_CHOWN="/nvim"
ENV DEV_CHOWNEXCLUDE="/nvim/build/pkgs"
WORKDIR /workspace
