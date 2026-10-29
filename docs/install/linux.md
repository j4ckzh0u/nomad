# Installing Nomad on Linux

Tested on Ubuntu 22.04 and 24.04, Debian 12, and a Rockchip arm64 board running
a 4.4 kernel. The client is the part that runs workloads; the server schedules
them. One process can be both, which is what this guide sets up.

## Requirements

| Item | Why |
| --- | --- |
| Linux 3.10 or later | The client needs cgroups and namespaces |
| Root, or `CAP_SYS_ADMIN` | `raw_exec` needs to signal arbitrary processes; task secrets use tmpfs |
| Ports 4646 (HTTP), 4647 (RPC), 4648 (Serf) free | Cluster communication |
| A writable data directory | Raft state, allocations, plugins |

The Docker driver additionally needs a reachable Docker socket. Nothing else
does, so a Docker-less host is a supported configuration, not a degraded one.

## Install

```sh
VERSION=2.1.1
ARCH=amd64      # or arm64
cd /tmp
curl -fsSLO "https://github.com/j4ckzh0u/nomad/releases/download/v${VERSION}/nomad_${VERSION}_linux_${ARCH}.zip"
unzip -o "nomad_${VERSION}_linux_${ARCH}.zip"
sudo install -m 0755 "nomad_${VERSION}_linux_${ARCH}" /usr/local/bin/nomad
nomad version
```

`install -m 0755` sets the mode explicitly, which avoids the umask surprises you
get from `cp` inside a restrictive shell.

## Prepare the host

```sh
sudo useradd --system --home /var/lib/nomad --shell /usr/sbin/nologin nomad
sudo mkdir -p /etc/nomad.d /var/lib/nomad
sudo chown nomad:nomad /var/lib/nomad
```

`raw_exec` runs tasks as the agent's user, so anything that needs root must run
as root. The alternative is to keep the agent as root and constrain jobs with
`user = "..."` in the job file, which is what most `raw_exec` deployments do.
Running the agent unprivileged is fine for container workloads.

## Configuration

`/etc/nomad.d/nomad.hcl`:

```hcl
data_dir  = "/var/lib/nomad"
bind_addr = "0.0.0.0"
log_level = "INFO"

server {
  enabled          = true
  bootstrap_expect = 1
}

client {
  enabled = true

  # raw_exec ships disabled. Enable it only on hosts you trust with root.
  options = {
    "driver.raw_exec.enable" = "1"
  }
}
```

For a cluster, set `bootstrap_expect` to the number of servers (three or five)
and list the peers:

```hcl
server {
  enabled          = true
  bootstrap_expect = 3
}

client {
  enabled = true
  servers = ["10.0.0.1:4647", "10.0.0.2:4647", "10.0.0.3:4647"]
}
```

## Run it as a service

`/etc/systemd/system/nomad.service`:

```ini
[Unit]
Description=Nomad
Documentation=https://developer.hashicorp.com/nomad
Wants=network-online.target
After=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/nomad agent -config /etc/nomad.d
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
KillSignal=SIGINT

LimitNOFILE=65536
LimitNPROC=infinity
TasksMax=infinity

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now nomad
systemctl status nomad
```

`Type=notify` relies on sd_notify support in the binary; if your service shows
as activating while Nomad is already up, switch it to `Type=simple`.

## Verify

```sh
nomad server members     # should list your node as alive
nomad node status        # should show the node as ready
nomad node status -self  # drivers reported, raw_exec among them
```

If `raw_exec` is missing from the driver list, the `options` block did not
apply. Check with:

```sh
nomad node status -verbose $(nomad node status -self -t '{{.ID}}') | grep -i raw
```

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
        args    = ["-c", "echo hello from $(hostname); sleep 5"]
      }
    }
  }
}
EOF

nomad job run /tmp/hello.nomad
nomad job status hello
nomad alloc logs -tail <alloc-id> t
```

More examples: [../examples/README.md](../examples/README.md).

## Firewall

| Port | Protocol | Purpose |
| --- | --- | --- |
| 4646 | TCP | HTTP API and UI |
| 4647 | TCP | RPC between servers and clients |
| 4648 | TCP/UDP | Serf gossip within a region |

Expose 4646 only where needed. The API is unauthenticated unless ACLs are on.

## Upgrade

Replacing the binary is enough; Raft state survives.

```sh
sudo systemctl stop nomad
sudo install -m 0755 nomad_2.1.2_linux_amd64 /usr/local/bin/nomad
sudo systemctl start nomad
nomad server members    # confirm the version reported by each node
```

Upgrade servers one at a time. A three-server cluster tolerates one restart.

## Uninstall

```sh
sudo systemctl disable --now nomad
sudo rm /etc/systemd/system/nomad.service /usr/local/bin/nomad
sudo rm -rf /etc/nomad.d /var/lib/nomad
```

Removing the data directory deletes all state. Drain the node first if there are
running allocations.

## Things that surprise people

- `-dev` writes to memory only. Every allocation is destroyed when the agent
  exits, and `job stop` is not needed.
- A node becomes `ready` before all drivers have fingerprinted. A job submitted
  in that window can fail feasibility with "missing drivers"; resubmit.
- `SIGTERM` to the agent does not stop allocations. That is deliberate, so a
  restart does not interrupt work. Use `nomad job stop` or `nomad node drain`.