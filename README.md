# Dev106

Development containers and configuration for classwork.

Currently supporting 6.106, 6.181, and 6.205.

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

Either way you need Docker running. If your daemon is colima, Rancher Desktop,
or rootless, see [SETUP.md](SETUP.md#docker-requirements) — dev106 does not read
Docker contexts. On Windows, run it inside WSL2 and keep your repos out of
`/mnt/c`; see [SETUP.md](SETUP.md#windows-and-wsl2).

## Usage

```bash
$ cd {some_assignment}
# first run writes ~/.config/dev106/config.toml, asking which course
# you're taking when a TTY is available, then continues
$ dev106 pull
$ dev106
dev106@64bf911d7f23:/workspace$ authorize-telerun
Enter your telerun credentials
Username: ^C
dev106@64bf911d7f23:/workspace$
logout
```

| command | |
|---|---|
| `dev106` | open a shell (same as `dev106 shell`) |
| `dev106 pull` | download the course image |
| `dev106 exec make -j4` | run one command in the container |
| `dev106 config` | show the effective config and where it came from |
| `dev106 list` | list dev106 containers |
| `dev106 kill` / `restart` | drop or recreate this repo's container |
| `dev106 nuke` | kill every dev106 container |

## Features

1. `dev106` finds your git repository root automatically. Invoke a shell from
   anywhere inside the repo and it seeks the root, which is mounted at
   `/workspace`.
2. Telerun credentials sync with the host. Run `authorize-telerun` in any
   dev106 shell, and then never again.
3. UID/GID preservation: inside the container you are the `dev106` user, but
   with the same UID and GID as on your host. No permission hiccups, and `sudo`
   works.
4. Taking more than one class? A `.dev106.toml` at a repo root overrides the
   global config for that repo alone.
5. For 6.2050, `lab-bc` credentials persist on the host and the FPGA board can
   be passed through for flashing. See
   [SETUP.md](SETUP.md#62050-lab-bc-and-the-fpga-board).

## More

[SETUP.md](SETUP.md) covers Docker daemon requirements, WSL2, the full config
reference, the course images, 6.2050 specifics, troubleshooting, and how to
write your own image.
