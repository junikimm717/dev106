# dev106 setup and troubleshooting

Everything that is not "install it and run it". See the [README](README.md)
for the quick start.

- [Docker requirements](#docker-requirements)
- [Windows and WSL2](#windows-and-wsl2)
- [Configuration](#configuration)
- [Course images](#course-images)
- [6.2050: lab-bc and the FPGA board](#62050-lab-bc-and-the-fpga-board)
- [Troubleshooting](#troubleshooting)
- [Writing your own image](#writing-your-own-image)

## Docker requirements

dev106 talks to the Docker socket directly. It reads `$DOCKER_HOST` and falls
back to `unix:///var/run/docker.sock`, but it does **not** read Docker
*contexts*.

Docker Desktop and OrbStack both provide the default socket and just work. If
you use colima, Rancher Desktop, or rootless Docker, only a context gets
registered, so you have to point `DOCKER_HOST` at the socket yourself:

```bash
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"     # colima
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"                 # rancher desktop
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"          # rootless
```

To find it for any setup:

```bash
docker context inspect -f '{{.Endpoints.docker.Host}}'
```

The installer checks the socket before it downloads anything and prints
whichever of these applies to you.

## Windows and WSL2

Run dev106 **inside WSL2**, not PowerShell. There is no Windows build; the
installer stops with instructions if you run it under MSYS/MinGW/Cygwin.

Keep your repos under your Linux home (`~`), **not** `/mnt/c`. WSL reaches
Windows drives over 9p (or virtiofs), so every `open` and `stat` is a round
trip out of the VM. Builds are many times slower, `chown` does not stick (so
dev106 cannot match your UID/GID), and file watching does not work. dev106
warns you if it detects a repo root on a Windows drive.

```bash
cd ~ && git clone <url>
```

Do not set `DOCKER_HOST=tcp://...` under WSL with Docker Desktop. Docker
Desktop rewrites bind mount paths in an API proxy sitting behind the distro's
unix socket; connecting over TCP skips that proxy, and `/workspace` silently
comes up empty instead of erroring. dev106 warns about this too.

## Configuration

The first run writes `~/.config/dev106/config.toml` (or
`$XDG_CONFIG_HOME/dev106/config.toml`) and then continues with whatever
command you typed. On a TTY it asks which course you are taking; without one it
defaults to 6.181. Cancelling the picker (`q` or Ctrl-C) writes nothing and
asks again next time.

| key | meaning |
|---|---|
| `image` | required; the container image to run |
| `telerun` | sync `~/.telerun` into the container. On for 6.106 |
| `follow_host` | use the host architecture instead of forcing linux/amd64 |
| `labbc` | persist lab-bc credentials on the host. 6.205 only |
| `usb` | pass the FPGA board through. 6.205 only |

### Taking more than one class

A `.dev106.toml` at the root of a repo overrides the global config for that
repo alone. Name only the keys that differ; everything else falls through to
the global file. So the global config stays on 6.181 and your 6.205 checkouts
opt themselves in:

```toml
# ~/6205/lab01/.dev106.toml
image = "ghcr.io/junikimm717/dev106/nvim_6205:latest"
labbc = true
usb = true
```

`dev106 config` prints the effective settings and which file each came from.

## Course images

| course | images | architectures |
|---|---|---|
| 6.1810 | `nvim_6181:latest`, `mit_6181:latest` | amd64, arm64 |
| 6.1060 | `nvim:2.1.0`, `nvim:4.0-rc1`, `mit_6106` | amd64 only |
| 6.2050 | `nvim_6205:latest`, `mit_6205:latest` | amd64, arm64 |

All live under `ghcr.io/junikimm717/dev106/`.

**6.181** includes QEMU 7.2+, gdb-multiarch, `riscv64-linux-gnu` GCC/binutils,
and a clangd preconfigured with the xv6 compile flags.

**6.106** is amd64-only; keep `follow_host = false` so it runs under emulation
on Apple Silicon.

**6.205** ships Icarus Verilog 12, cocotb, pyserial, openFPGALoader, vicoco,
and `lab-bc`, all on `PATH`. They live in a venv at `/opt/6205_python`, which
is owned by root so that starting a container does not mean chowning every
file in it. Adding a package therefore needs sudo, which you have:

```bash
sudo pip install <package>
```

## 6.2050: lab-bc and the FPGA board

Vivado is not installed and never will be — it is ~90 GB. Builds go to the
course's build server through `lab-bc`, as the
[course docs](https://fpga.mit.edu/6205/F26/documentation/vivado) describe.

```toml
labbc = true
usb = true
```

### lab-bc credentials

`labbc = true` bind-mounts `~/.config/lab-bc` from the host, so `lab-bc
configure` is a one-time step rather than once per container. Your kerberos and
MIT ID stay on your machine, never in the image, and survive `dev106 restart`
and `dev106 nuke`.

```bash
$ dev106
dev106@8f2c1a:/workspace$ lab-bc configure     # once, ever
dev106@8f2c1a:/workspace$ lab-bc build ./ build.tcl
```

### USB passthrough

`usb = true` passes the Urbana board (FTDI FT2232H, `0403:6010`) in for
flashing and UART. dev106 mounts `/dev/bus/usb`, adds a cgroup rule for the USB
device major so replugging keeps working without recreating the container, maps
any serial nodes in, and adds the host groups owning those nodes to your
container user.

Whether this works depends on your **Docker daemon**, not your OS — the devices
a container sees are the daemon's. dev106 asks the daemon which it is and
adapts:

| daemon | flashing | what you have to do first |
|---|---|---|
| Docker Engine on Linux | yes | nothing; the board is scanned and passed through |
| Docker Desktop under WSL2 | yes | `usbipd attach --wsl --hardware-id 0403:6010` |
| OrbStack (macOS) | yes | `orb usb attach <id>`; serial adapters forward automatically |
| Docker Desktop (macOS/Windows) | no | — |
| remote `DOCKER_HOST` | no | — |

On a Mac, **OrbStack can flash and Docker Desktop cannot.** Docker Desktop
reaches USB only over USB/IP, which needs a privileged helper container held
open for the session and does not present the board at `/dev/bus/usb`; on macOS
there is no complete USB/IP server anyway. Either switch to OrbStack or build
the bitstream in the container and flash from a Linux host.

None of this blocks you. Simulation (iverilog, cocotb) and `lab-bc build` need
no board at all — only flashing and UART do.

### Editor tooling

The `nvim_6205` image ships verible, svlangserver, pyright, and ruff, with
verilator behind svlangserver. If you use your own editor, the table below is
the part worth copying — the tools matter more than the config.

No single SystemVerilog tool catches everything, because most of them parse
rather than elaborate. Measured against one file with five deliberate
mistakes:

| mistake | verible | iverilog | verilator |
|---|---|---|---|
| blocking assignment in `always_ff` | yes | no | yes |
| `case` with no default (inferred latch) | yes | no | yes |
| undeclared signal (`typo_signal`) | no | yes | yes |
| instantiating a nonexistent module | no | yes | yes |
| width mismatch (8-bit into `logic [3:0]`) | no | no | yes |

**verilator does the heavy lifting.** `verilator --sv --lint-only -Wall` is
the only one that elaborates, so it is the only one that sees width
truncation, unused signal bits, and undriven logic. It is not part of the
course toolchain; dev106 installs it anyway.

**Language servers, ranked by how much they actually help:**

- **svlangserver** — indexes the project for cross-module go-to-definition,
  and shells out to `verilator --lint-only` for diagnostics. Installing it
  without verilator on `PATH` gets you navigation and silence. npm package, so
  it works on any architecture.
- **verible** — ChipsAlliance. Style and syntax only, but its rules are good
  ones and its messages explain themselves. Also gives you
  `verible-verilog-format`. Prebuilt for linux x64 and arm64.
- **svls** — uses svlint for diagnostics. Reasonable, but mason builds it from
  source with cargo, so you need a Rust toolchain.
- **veridian** — not in the mason registry; build it yourself.
- **hdl_checker** — wraps ghdl/vcom/xvhdl. Useless unless you already have one
  of those.
- **slangd / mason's `slang`** — **not this one.** That is the NVIDIA *shading*
  language. The SystemVerilog project also called slang is unrelated and is not
  what these install.

Running verible and svlangserver together is the useful combination: style
rules from one, elaboration errors from the other. They overlap on a couple of
rules, so expect the occasional doubled diagnostic.

For Python, **pyright** plus **ruff**. Not basedpyright — cocotb's `dut` handle
is dynamically typed, and strict mode reports a dozen unknown-type warnings on
a trivial testbench, which buries the real errors.

If your editor reports `Import "cocotb" could not be resolved`, it is resolving
the wrong interpreter. The toolchain lives in a venv at `/opt/6205_python`, and
the image sets `VIRTUAL_ENV` so editors find it; set that yourself if you are
building your own image.

## Troubleshooting

**`could not connect to Docker` / `Cannot connect to the Docker daemon`**
The daemon is not running, or it is listening somewhere dev106 is not looking.
See [Docker requirements](#docker-requirements).

**`not inside a git repository`**
dev106 names containers after the repo root, so it needs one. `cd` into a
clone. `dev106 pull`, `list`, `nuke`, and `config` work anywhere.

**`image ... is not installed locally`**
Run `dev106 pull`.

**`container ... already exists`**
`dev106` to attach, or `dev106 restart` to recreate it.

**Builds are absurdly slow, or `chown` does not stick**
You are probably on `/mnt/c` under WSL. See [Windows and WSL2](#windows-and-wsl2).

**`/workspace` is empty**
Under WSL with Docker Desktop, check that `DOCKER_HOST` is not set to a
`tcp://` address. See [Windows and WSL2](#windows-and-wsl2).

**openFPGALoader cannot find the board**
Check the table in [USB passthrough](#usb-passthrough) first. If your daemon
can flash and it still fails:

- The board must be attached before the container is created. Plug it in, then
  `dev106 restart` — a container's device list is fixed at creation.
- On Linux, a board owned by root means the udev rule is missing:
  ```bash
  sudo curl -fsSL -o /etc/udev/rules.d/99-openfpgaloader.rules \
    https://raw.githubusercontent.com/trabucayre/openFPGALoader/master/99-openfpgaloader.rules
  sudo udevadm control --reload-rules && sudo udevadm trigger
  ```
  Then replug the board.

**xv6: gdb connects to nothing, or rejects `riscv:rv64`**
The 6.181 image handles both of these, so if you hit them you are on an image
built before this was fixed — re-pull. `make qemu-gdb` says to run `gdb`, but
Debian's plain gdb is native-only; the image points `gdb` at `gdb-multiarch`,
which speaks RISC-V. The same target writes a `.gdbinit` at the repo root
carrying your port and `target remote`, and gdb refuses to auto-load a gdbinit
outside its safe path, which leaves you attached to nothing. The image trusts
`/workspace` for exactly that reason. Outside dev106, the equivalent is:

```bash
echo "add-auto-load-safe-path $(pwd)" >> ~/.config/gdb/gdbinit
```

**A power cycle killed the UART**
Serial nodes only exist while the board is plugged in. `dev106 restart`.

## Writing your own image

The home directory of the dev106 user is hard coded to `/home/dev106`. Think
from that user's perspective, not root's.

`./cmd/bootstrap` is the container entrypoint. It rewrites `/etc/passwd`,
`/etc/group`, and `/etc/shadow` so the container user matches your host UID and
GID, which is why you never hit permission problems on bind-mounted files.

Set by dev106 at runtime:

- `DEV_UID` — your UID on the host. Nonnegative integer.
- `DEV_GID` — your GID on the host. Nonnegative integer.

Set by you, the image author:

- `DEV_CHOWN` — colon-separated directories to recursively chown at startup
  (e.g. `/nvim:/dir/dir2`).
- `DEV_CHOWNEXCLUDE` — colon-separated directories to skip during that chown.
  Use it on large artifact directories that never need writing; they otherwise
  add real startup cost.

What happens on launch:

1. The CLI creates the container.
2. The bootstrapper rewrites ownership using the env vars above.
3. The CLI execs a shell with the same UID and GID as you.

Also set `WORKDIR /workspace`. The repo root is bind-mounted there, and without
it your shell starts in `/`.

### Sample

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
