# Installing Nomad on macOS

macOS is a good place to run a single-node agent or a dev cluster. It is not a
production server platform: there are no cgroups, no bridge networking, and no
`exec` driver.

Verified on macOS 14 and 15, both architectures.

## Requirements

| Item | Why |
| --- | --- |
| macOS 12 or later | The binaries are built with a recent Go toolchain |
| The archive matching your CPU | `darwin_arm64` for Apple Silicon, `darwin_amd64` for Intel |
| Admin rights | Only to install into `/usr/local/bin` |

## Install

```sh
VERSION=2.1.1
ARCH=arm64      # or amd64
cd ~/Downloads
curl -fsSLO "https://github.com/j4ckzh0u/nomad/releases/download/v${VERSION}/nomad_${VERSION}_darwin_${ARCH}.zip"
unzip -o "nomad_${VERSION}_darwin_${ARCH}.zip"
sudo install -m 0755 "nomad_${VERSION}_darwin_${ARCH}" /usr/local/bin/nomad
nomad version
```

### Gatekeeper

The archives are not signed or notarised, so a downloaded binary is quarantined
and macOS refuses to run it:

```
"nomad" cannot be opened because the developer cannot be verified.
```

Clear the attribute once:

```sh
xattr -d com.apple.quarantine /usr/local/bin/nomad
```

If you already ran the binary and the dialog appeared, the same command fixes
it. `xattr -l /usr/local/bin/nomad` shows whether the attribute is present.

Building from source avoids the issue entirely and produces a binary macOS
trusts locally:

```sh
go build -o nomad .
```

## Configuration

For a single node, `~/.config/nomad/nomad.hcl`:

```hcl
data_dir  = "/usr/local/var/nomad"
bind_addr = "127.0.0.1"
log_level = "INFO"

server {
  enabled          = true
  bootstrap_expect = 1
}

client {
  enabled = true
  options = {
    "driver.raw_exec.enable" = "1"
  }
}
```

`bind_addr = "127.0.0.1"` keeps the API local. If you need it reachable from
other machines, use `0.0.0.0` and read the firewall section below.

The default data directory is under the agent's working directory. Set it
explicitly, or state ends up somewhere you will not find later.

## Run it

Foreground, which is enough for testing:

```sh
nomad agent -config ~/.config/nomad/nomad.hcl
```

Development mode, with everything in memory:

```sh
nomad agent -dev
```

`-dev` creates its own data directory, runs one server and one client, and
destroys state on exit. It is the fastest way to try a job.

### Run it at login (launchd)

`~/Library/LaunchAgents/io.nomad.agent.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>io.nomad.agent</string>

  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/nomad</string>
    <string>agent</string>
    <string>-config</string>
    <string>/Users/CHANGEME/.config/nomad/nomad.hcl</string>
  </array>

  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>

  <key>StandardOutPath</key>
  <string>/usr/local/var/log/nomad.log</string>
  <key>StandardErrorPath</key>
  <string>/usr/local/var/log/nomad.err</string>
</dict>
</plist>
```

```sh
mkdir -p /usr/local/var/log /usr/local/var/nomad
launchctl load ~/Library/LaunchAgents/io.nomad.agent.plist
launchctl list | grep nomad
```

Replace `CHANGEME` with your username; launchd does not expand `~` inside
`ProgramArguments`.

Unload with `launchctl unload ~/Library/LaunchAgents/io.nomad.agent.plist`.

## Verify

```sh
nomad server members
nomad node status
nomad node status -self
```

The driver list will show `raw_exec` and `qemu` if QEMU is installed. `docker`
appears only when a Docker daemon is reachable, which on macOS means Docker
Desktop or Colima running.

## First job

```sh
cat > /tmp/hello.nomad <<'EOF'
job "hello" {
  datacenters = ["dc1"]
  type        = "batch"

  group "g" {
    task "t" {
      driver = "raw_exec"
      config {
        command = "/bin/sh"
        args    = ["-c", "echo hello from $(hostname)"]
      }
    }
  }
}
EOF

nomad job run /tmp/hello.nomad
nomad job status hello
```

More examples: [../examples/README.md](../examples/README.md).

## Firewall and reachability

macOS does not enable a firewall by default; if you turned it on, allow
incoming connections for the binary or the ports will be blocked silently.

| Port | Protocol | Purpose |
| --- | --- | --- |
| 4646 | TCP | HTTP API and UI |
| 4647 | TCP | RPC |
| 4648 | TCP/UDP | Serf gossip |

## What does not work here

| Feature | Status |
| --- | --- |
| `exec` driver | Unavailable. It needs Linux cgroups and chroot |
| Bridge networking, CNI | Unavailable. Use `network { mode = "host" }` |
| cgroup-based resource limits | Not enforced; values are scheduling hints only |
| Task user (`user = "..."`) | Only as the current user |
| CSI volumes with host path plugins | Works, but the plugin must be a macOS build |

`raw_exec` and `qemu` are the useful drivers. Docker works when a daemon is
running, subject to the usual Docker Desktop path sharing rules.

## Uninstall

```sh
launchctl unload ~/Library/LaunchAgents/io.nomad.agent.plist
sudo rm /usr/local/bin/nomad
rm -rf /usr/local/var/nomad ~/.config/nomad
```

## Things that surprise people

- The first run of an unsigned binary fails with a Gatekeeper dialog, not a
  shell error. Check `xattr -l` before assuming the binary is broken.
- Apple Silicon will not run `darwin_amd64` under Rosetta unless it is
  installed; `Bad CPU type in executable` means you downloaded the wrong
  archive.
- `-dev` binds to localhost only. Remote access needs an explicit `bind_addr`.