# Dev106

Development containers and configuration for classwork.

Currently supporting 6.106, 6.181, and 6.205

## Installation

If you are lazy:

```bash
curl -fsSL https://raw.githubusercontent.com/junikimm717/dev106/master/install.sh | sh
```

Checks Docker, installs the binary, then runs `dev106 pull` to pick your course
and download the image. macOS, Linux, and WSL2. Re-run it to upgrade. It
explains what to do if anything is missing; `sh install.sh --help` lists the
knobs.

Or, from source:

```bash
go install github.com/junikimm717/dev106@latest
```

Either way you need Docker running. dev106 talks to the Docker socket directly
and reads `$DOCKER_HOST`, but it does **not** read Docker *contexts*. Docker
Desktop and OrbStack provide `unix:///var/run/docker.sock` and just work;

If you're using colima, rancher desktop, or rootless docker, I assume you are
serious and know what you are doing:
```bash
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"  # colima
```

On Windows, run dev106 inside WSL2 rather than PowerShell, and keep your repos
under your Linux home (`~`) instead of `/mnt/c`.

The first run writes a config at `~/.config/dev106/config.toml` (or
`$XDG_CONFIG_HOME/dev106/config.toml`) and then continues with the command you
typed. On a TTY it asks which course you are taking; without a TTY it defaults
to 6.181. If you cancel the picker (`q` or Ctrl-C), nothing is written and
dev106 asks again the next time you run it. You can still edit the file
afterward.

```toml
image = "ghcr.io/junikimm717/dev106/nvim_6181:latest"
telerun = false
# Use the host architecture (arm64/amd64). On for 6.181 and 6.205, off for 6.106.
follow_host = true
# 6.205 only; see below.
labbc = false
usb = false
```

**6.181** images (`nvim_6181:latest`, `mit_6181:latest`) are built for amd64 and
arm64. They include QEMU 7.2+, gdb-multiarch, and `riscv64-linux-gnu` GCC/binutils.

**6.106** images (`nvim:2.1.0`, `nvim:4.0-rc1`, `mit_6106`) are amd64-only; keep
`follow_host = false` so they run under emulation on Apple Silicon.

**6.205** images (`nvim_6205:latest`, `mit_6205:latest`) are amd64 and arm64.
See [6.2050](#62050) below.

### Taking more than one class

Drop a `.dev106.toml` at the root of a repo and it overrides the global config
for that repo alone. Only name the keys that differ; everything else falls
through to the global file. So the global config stays on 6.181 while your
6.205 checkouts opt themselves in:

```toml
# ~/6205/lab01/.dev106.toml
image = "ghcr.io/junikimm717/dev106/nvim_6205:latest"
labbc = true
usb = true
```

`dev106 config` prints the effective settings and which file each came from.


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
$ dev106 list
$ dev106 config
$ dev106 kill
$ dev106 restart
$ dev106 nuke
```

## 6.2050

Vivado is not installed and never will be — it is ~90 GB. Builds go to the
course's build server through `lab-bc`, exactly as the
[course docs](https://fpga.mit.edu/6205/F26/documentation/vivado) describe. The
image ships Icarus Verilog 12, cocotb, pyserial, openFPGALoader, vicoco, and
`lab-bc`, all on `PATH`.

```toml
labbc = true
usb = true
```

`labbc = true` bind-mounts `~/.config/lab-bc` from the host, so `lab-bc
configure` is a one-time step. Your kerberos and MIT ID live on your machine,
not in the image, and survive `dev106 restart` and `dev106 nuke`.

```bash
$ dev106
dev106@8f2c1a:/workspace$ lab-bc configure     # once, ever
dev106@8f2c1a:/workspace$ lab-bc build ./ build.tcl
```

`usb = true` passes the Urbana board (FTDI FT2232H, `0403:6010`) into the
container for flashing and UART: `/dev/bus/usb` is mounted, a cgroup rule for
USB devices is added so replugging keeps working, any `/dev/ttyUSB*` and
`/dev/ttyACM*` are mapped in, and the host groups owning those nodes are added
to your container user.

Whether this works depends on your **Docker daemon**, not your OS — the
devices a container sees are the daemon's. dev106 asks the daemon which it is
and adapts:

| daemon | flashing | notes |
|---|---|---|
| Docker Engine on Linux | yes | the board is scanned and passed straight through |
| Docker Desktop under WSL2 | yes | `usbipd attach --wsl` first; WSL2 distros share one kernel |
| OrbStack (macOS) | yes | `orb usb attach` first; serial adapters forward automatically |
| Docker Desktop (macOS/Windows) | no | reaches USB only over USB/IP, which needs a privileged helper container and does not present `/dev/bus/usb` |
| remote `DOCKER_HOST` | no | the board would have to be plugged into the daemon's machine |

So on a Mac, **OrbStack can flash and Docker Desktop cannot**. If you're on
Docker Desktop for Mac, either switch to OrbStack or build the bitstream here
and flash from a Linux host.

dev106 says which of these you're in rather than letting openFPGALoader fail
later — no board plugged in, a board owned by root (missing udev rule), or a
board plugged in after the container was created (`dev106 restart`, since a
container's devices are fixed at creation). None of it blocks you; simulation
and `lab-bc build` need no board at all.

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
