# Installation

## Release binary (recommended)

Every release ships a static binary per platform — `cidx-linux-amd64`,
`cidx-linux-arm64`, `cidx-darwin-amd64`, `cidx-darwin-arm64`,
`cidx-windows-amd64.exe` — and a `checksums.txt`. Nothing to build, no
toolchain to install.

```bash
curl -fsSLO https://github.com/cidx-org/cidx/releases/latest/download/cidx-linux-amd64
curl -fsSL  https://github.com/cidx-org/cidx/releases/latest/download/checksums.txt | sha256sum -c --ignore-missing
sudo install -m 755 cidx-linux-amd64 /usr/local/bin/cidx
cidx --version
```

Replace `linux-amd64` with your platform. On macOS, use
`shasum -a 256 -c --ignore-missing` for the checksum step. To pin a version
rather than track `latest`, download from
`https://github.com/cidx-org/cidx/releases/download/v3.4.1/cidx-linux-amd64`.

Docker or Podman must be installed and running: cidx runs every tool in a
container and never installs tools on your host.

## Via Go install

With Go 1.26 or newer (an older toolchain fetches it when `GOTOOLCHAIN=auto`):

```bash
go install github.com/cidx-org/cidx/v3/cmd/cidx@latest
```

Mind the `/v3` in the module path. Since v3.0.0 the module lives there, and
the bare `github.com/cidx-org/cidx/cmd/cidx@latest` resolves to the last
pre-v3 module version, v1.8.0.

## From source

```bash
git clone https://github.com/cidx-org/cidx.git
cd cidx
go build -o bin/cidx ./cmd/cidx
```
