#在 macOS上安装 Nomad

macOS适合跑单节点或开发集群。它**不是**生产服务端平台：没有 cgroups、
没有桥接网络、也没有 `exec`驱动。

在 macOS 14和15、两种架构上都验证过。

##环境要求

|项目 |原因 |
| --- | --- |
| macOS 12及以上 |二进制用较新的 Go工具链构建 |
|与 CPU匹配的压缩包 | Apple Silicon选 `darwin_arm64`，Intel选 `darwin_amd64` |
|管理员权限 |仅用于安装到 `/usr/local/bin` |

##安装

```sh
VERSION=2.1.1
ARCH=arm64 # Intel机器用amd64
cd ~/Downloads
curl -fsSLO "https://github.com/j4ckzh0u/nomad/releases/download/v${VERSION}/nomad_${VERSION}_darwin_${ARCH}.zip"
unzip -o "nomad_${VERSION}_darwin_${ARCH}.zip"
sudo install -m 0755 "nomad_${VERSION}_darwin_${ARCH}" /usr/local/bin/nomad
nomad version
```

###Gatekeeper

压缩包未签名、未公证，从网上下载的二进制会被打上隔离标记，macOS拒绝执行：

```
"nomad" cannot be opened because the developer cannot be verified.
```

清一次属性即可：

```sh
xattr -d com.apple.quarantine /usr/local/bin/nomad
```

用 `xattr -l /usr/local/bin/nomad`可以确认属性是否还在。

从源码构建则完全不会遇到这个问题：

```sh
go build -o nomad .
```

##配置

单节点，`~/.config/nomad/nomad.hcl`：

```hcl
data_dir = "/usr/local/var/nomad"
bind_addr = "127.0.0.1"
log_level = "INFO"

server {
 enabled = true
 bootstrap_expect = 1
}

client {
 enabled = true
 options = {
 "driver.raw_exec.enable" = "1"
 }
}
```

`bind_addr = "127.0.0.1"`让 API只在本机可达。需要从其他机器访问时改成
`0.0.0.0`，并注意下面的防火墙一节。

默认数据目录位于 agent的工作目录之下。**请显式设置 `data_dir`**，
否则状态文件会落到你之后找不到的地方。

##运行

前台运行，测试够用：

```sh
nomad agent -config ~/.config/nomad/nomad.hcl
```

开发模式，全部状态在内存：

```sh
nomad agent -dev
```

`-dev`会自建数据目录、在单进程内跑一个服务端和一个客户端，退出即销毁状态。
试跑任务用这个最快。

###开机登录时启动（launchd）

`~/Library/LaunchAgents/io.nomad.agent.plist`：

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

把 `CHANGEME`换成你的用户名。launchd**不会**在 `ProgramArguments`里展开 `~`，
必须写绝对路径。

停止加载：`launchctl unload ~/Library/LaunchAgents/io.nomad.agent.plist`。

##验证

```sh
nomad server members
nomad node status
nomad node status -self
```

驱动列表里会出现 `raw_exec`；装了 QEMU还有 `qemu`。只有能连上 Docker守护进程
（Docker Desktop或 Colima在跑）时才会出现 `docker`。

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
 args = ["-c", "echo hello from $(hostname)"]
 }
 }
 }
}
EOF

nomad job run /tmp/hello.nomad
nomad job status hello
```

更多示例见 [../examples/README.md](../examples/README.md)。

##防火墙与可达性

macOS默认不开防火墙。若你打开了，需要放行该二进制或对应端口，
否则连接会被静默丢弃。

|端口 |协议 |用途 |
| --- | --- | --- |
|4646 | TCP | HTTP API与 Web UI |
|4647 | TCP | RPC |
|4648 | TCP/UDP | Serf gossip |

##这里跑不了的功能

|功能 |状态 |
| --- | --- |
| `exec`驱动 |不可用，依赖 Linux的 cgroups和 chroot |
|桥接网络、CNI |不可用，请用 `network { mode = "host" }` |
| cgroup资源限制 |不生效，数值仅作调度参考 |
|任务用户（`user = "..."`）|只能以当前用户身份运行 |
| CSI主机路径插件 |可用，但插件需要 macOS构建 |

实际有用的是 `raw_exec`和 `qemu`。Docker在有守护进程时可用，
受 Docker Desktop的路径共享规则限制。

##卸载

```sh
launchctl unload ~/Library/LaunchAgents/io.nomad.agent.plist
sudo rm /usr/local/bin/nomad
rm -rf /usr/local/var/nomad ~/.config/nomad
```

##容易踩的坑

-未签名的二进制第一次运行是被 Gatekeeper弹窗拦住，shell里看不到错误。
先查 `xattr -l`，不要以为二进制坏了。
-Apple Silicon上直接跑 `darwin_amd64`会报 `Bad CPU type in executable`，
这意味着下错了压缩包；除非装了 Rosetta。
- `-dev`只监听 localhost。要从远程访问必须显式设置 `bind_addr`。
