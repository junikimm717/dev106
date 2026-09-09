# Dev106
6.106 development containers and configuration for neovim users.

## Features:

1. `dev106` automatically detects your git repository root. You can invoke a
shell from anywhere inside your repo and it will seek the repository root.
2. Telerun configuration gets synced with the host. Run `authorize-telerun`
  in any dev106 shell, and then never again.
3. UID/GID preservation; when you exec into a dev106 container, you are a
`dev106` user, but with the same uid and gid as on your host machine. No
permission hiccups. `sudo` works automatically (provided you have a good image).

```bash
$ cd {some_assignment}
# first run writes ~/.config/dev106/config.toml (or $XDG_CONFIG_HOME/dev106)
# after asking 6.181 vs 6.106 when a TTY is available, then continues
$ dev106 pull
$ dev106
dev106@64bf911d7f23:/workspace$ authorize-telerun
Enter your telerun credentials
Username: ^C
dev106@64bf911d7f23:/workspace$
logout

$ dev106 exec make -j4
$ dev106 kill
$ dev106 restart
```

## Installation

Download a pre-release binary from the
[nightly release](https://github.com/junikimm717/dev106/releases/tag/nightly),
or install from source:

```bash
go install github.com/junikimm717/dev106@latest
# run the binary, this should work if ~/go/bin is in your $PATH
dev106
```

The first run writes a config at `~/.config/dev106/config.toml` (or
`$XDG_CONFIG_HOME/dev106/config.toml`) and then continues with the command you
typed. On a TTY it asks whether you are taking 6.181 or 6.106; without a TTY it
defaults to 6.181. You can still edit the file afterward.

```toml
image = "ghcr.io/junikimm717/dev106/nvim_6181:latest"
telerun = true
# Use the host architecture (arm64/amd64). On for 6.181, off for 6.106.
follow_host = true
```

**6.106** images (`nvim:2.1.0`, `nvim:4.0-rc1`, `mit_6106`) are amd64-only; keep
`follow_host = false` so they run under emulation on Apple Silicon.

**6.181** images (`nvim_6181:latest`, `mit_6181:latest`) are built for amd64 and
arm64. They include QEMU 7.2+, gdb-multiarch, and `riscv64-linux-gnu` GCC/binutils.

## Container Bootstrapper

**Important**: The home directory of the dev106 user is hard coded to be
`/home/dev106`. This is important to note when you create docker images, as you
should think from the perspective of the dev106 user, not the root user.

There is a container entrypoint in `./cmd/bootstrap` (in this source code
tree) that works to set the UID and GID of the running docker process to match
your machine. It accepts the following environment variables:

- `DEV_UID` - your UID on your host machine. Must be a nonnegative integer.
- `DEV_GID` - your GID on your host machine. Must be a nonnegative integer.

Environment variables configured by the Dockerfile authors:

- `DEV_CHOWN` - a colon-separated list of directories to run a recursive chown
on (e.g. `/nvim:/dir/dir2`); the image authors should be responsible for this.
- `DEV_CHOWNEXCLUDE` - a colon separated list of directories to exclude from
chowning when doing the recursive chown operation. You should use this on
massive artifact directories that don't actually need to be written to.

In short, here is the flow of what happens when you launch a dev106 container:
1. The CLI launches a dev106 container.
2. The bootstrapper uses the env vars above to rewrite the owner UID and GID's
   on the container filesystem
3. The CLI starts a shell into the dev106 container with the same UID and GID as
   the user.

## Image Sample

Please reference this image when creating your own docker images.

I'll hopefully create a github repo soon with a sample Dockerfile and GitHub
actions configuration.

```dockerfile
# this image already has the entire 6.106 toolchain installed.
FROM ghcr.io/junikimm717/dev106/mit_6106:latest

# >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
# Neovim Installation and Configuration

RUN mkdir -p /home/dev106/.config/nvim && \
  curl -L https://github.com/junikimm717/nvim2025/archive/master.tar.gz\
  | tar -xz --strip-components=1 -C /home/dev106/.config/nvim
# this is a custom runtime configuration file for juni's neovim setup, ignore
# for your own config.
RUN <<EOF cat > /home/dev106/.config/nvim/lua/configs/init.lua
---@type Config
return {
  mason = {},
  lazy = require("themes.kanagawa"),
  treesitter = { "c", "make", "bash", "asm" }
}
EOF
RUN cd /home/dev106/.config/nvim && make -j$(nproc)
RUN /home/dev106/.config/nvim/build/bin/setup-nvim
# <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<

# >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
# Home Directory Configuration

# you almost certainly want opencilk's bin directory first (so that you have
# access to their clangd lsp)
RUN <<EOF cat > /home/dev106/.profile
# I like using vim mode
set -o vi
export PATH=/opt/6106/opencilk/bin:$PATH
export PATH=/nvim/build/bin:$PATH
EOF
# <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<

# >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
# Env Var Configuration

# Values for the two environment variables below are formatted as
# /my/dir/1:/my/dir2:... (colon-separated)

# if you have directories outside of /home/dev106 that need to be chowned to be
# owned by dev106 at runtime, do that here.
ENV DEV_CHOWN=""

# There is no need to chown these packages, and they take up significant
# overhead on runtime startup
ENV DEV_CHOWNEXCLUDE="/home/dev106/.config/nvim/build/pkgs"
# <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<

# We recommend you keep this option as-is.
WORKDIR /workspace
```
