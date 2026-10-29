#在 Linux上安装 Nomad

在 Ubuntu 22.04/24.04、Debian 12，以及一块运行4.4内核的 Rockchip arm64开发板上
验证过。客户端负责跑负载，服务端负责调度，一个进程可以同时是两者，本文按
这种方式安装。

##环境要求

|项目 |原因 |
| --- | --- |
| Linux 3.10及以上 |客户端需要 cgroups和命名空间 |
| root或 `CAP_SYS_ADMIN` | `raw_exec`需要给任意进程发信号；任务密钥目录用 tmpfs |
|4646（HTTP）、4647（RPC）、4648（Serf）端口空闲 |集群通信 |
|可写的数据目录 |存放 Raft状态、分配目录、插件 |

Docker驱动额外需要一个可连通的 Docker socket。其他模块都不需要，所以
**没有 Docker的机器是受支持的配置，不是降级状态**。

##安装

```sh
VERSION=2.1.1
ARCH=amd64 # arm64为ARM服务器
cd /tmp
curl -fsSLO "https://github.com/j4ckzh0u/nomad/releases/download/v${VERSION}/nomad_${VERSION}_linux_${ARCH}.zip"
unzip -o "nomad_${VERSION}_linux_${ARCH}.zip"
sudo install -m 0755 "nomad_${VERSION}_linux_${ARCH}" /usr/local/bin/nomad
nomad version
```

用 `install -m 0755`而不是 `cp`，可以避开 umask导致的权限差异。

##准备主机

```sh
sudo useradd --system --home /var/lib/nomad --shell /usr/sbin/nologin nomad
sudo mkdir -p /etc/nomad.d /var/lib/nomad
sudo chown nomad:nomad /var/lib/nomad
```

`raw_exec`以 agent的用户身份运行任务，所以需要 root的任务必须让 agent以
root运行；另一种做法是 agent保持 root，在 job里用 `user = "..."`限制
任务身份。agent非特权运行对容器类负载完全够用。

##配置

`/etc/nomad.d/nomad.hcl`：

```hcl
data_dir = "/var/lib/nomad"
bind_addr = "0.0.0.0"
log_level = "INFO"

server {
 enabled = true
 bootstrap_expect = 1
}

client {
 enabled = true

 # raw_exec默认关闭。只在信任其 root权限的主机上开启。
 options = {
 "driver.raw_exec.enable" = "1"
 }
}
```

集群模式下把 `bootstrap_expect`设成服务端数量（3或5），并列出对端：

```hcl
server {
 enabled = true
 bootstrap_expect = 3
}

client {
 enabled = true
 servers = ["10.0.0.1:4647", "10.0.0.2:4647", "10.0.0.3:4647"]
}
```

##作为服务运行

`/etc/systemd/system/nomad.service`：

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

LimitNOFILE= 65536
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

`Type=notify`依赖二进制里的 sd_notify支持。如果 Nomad已经起来了但服务状态
一直显示 activating，把它改成 `Type=simple`。

##验证

```sh
nomad server members #应列出你的节点且状态为 alive
nomad node status #节点应为 ready
nomad node status -self #显示识别到的驱动，应包含 raw_exec
```

如果驱动列表里没有 `raw_exec`，说明 `options`块没生效。检查方式：

```sh
nomad node status -verbose $(nomad node status -self -t '{{.ID}}') | grep -i raw
```

##第一个任务

```sh
cat > /tmp/hello.nomad <<'EOF'
job "hello" {
 datacenters = ["dc1"]
 type = "batch"

 group "g" {
 task "t" {
 driver = "raw_exec"
 config {
 command = "/bin/sh"
 args = ["-c", "echo hello from $(hostname); sleep 5"]
 }
 }
 }
}
EOF

nomad job run /tmp/hello.nomad
nomad job status hello
nomad alloc logs -tail <分配ID> t
```

更多示例见 [../examples/README.md](../examples/README.md)。

##防火墙

|端口 |协议 |用途 |
| --- | --- | --- |
|4646 | TCP | HTTP API与 Web UI |
|4647 | TCP |服务端与客户端之间的 RPC |
|4648 | TCP/UDP |区域内的 Serf gossip |

4646只对需要的网段开放。**未启用 ACL时，该 API没有任何认证。**

##升级

替换二进制即可，Raft状态会保留。

```sh
sudo systemctl stop nomad
sudo install -m 0755 nomad_2.1.2_linux_amd64 /usr/local/bin/nomad
sudo systemctl start nomad
nomad server members #确认各节点上报的版本
```

服务端逐个升级。三节点集群可以容忍一次重启。

##卸载

```sh
sudo systemctl disable --now nomad
sudo rm /etc/systemd/system/nomad.service /usr/local/bin/nomad
sudo rm -rf /etc/nomad.d /var/lib/nomad
```

删除数据目录会清空全部状态。如果还有运行中的分配，先 drain节点。

##容易踩的坑

- `-dev`只写内存。agent退出时所有分配都会被销毁，不需要手动 `job stop`。
-节点变成 `ready`时，驱动可能还没完成指纹采集。这个窗口内提交任务会因
 "缺少驱动"而可行性检查失败，重提一次即可。
-给 agent发 `SIGTERM`**不会**停止分配。这是刻意设计，避免重启中断业务。
要停负载请用 `nomad job stop`或 `nomad node drain`。
