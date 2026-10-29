#在 Windows上安装 Nomad

Windows作为 Nomad客户端支持良好。服务端也能跑，适合搭实验环境；
生产集群通常把服务端放在 Linux上。

在 Windows 10、11和 Server 2022上验证过。

##环境要求

|项目 |原因 |
| --- | --- |
| Windows 10或 Server 2016及以上 |二进制只提供 x86_64 |
|管理员 PowerShell |注册服务、开放防火墙端口 |
|4646、4647、4648端口空闲 |集群通信 |

下面的命令必须在**管理员** PowerShell里执行。`Start-Process -Verb RunAs`
不足以完成服务注册那一步。

##安装

```powershell
$Version = '2.1.1'
$Url = "https://github.com/j4ckzh0u/nomad/releases/download/v$Version/nomad_${Version}_windows_amd64.exe"
$Dest = 'C:\Program Files\Nomad'

New-Item -ItemType Directory -Force -Path $Dest | Out-Null
Invoke-WebRequest -Uri $Url -OutFile "$Dest\nomad.exe"

#从网络下载的文件带"网络标记"，首次执行会弹窗拦截。清一次即可。
Unblock-File "$Dest\nomad.exe"

# CLI需要安装目录在 PATH里
[Environment]::SetEnvironmentVariable(
 'Path',
 [Environment]::GetEnvironmentVariable('Path', 'Machine') + ";$Dest",
 'Machine'
)
$env:Path += ";$Dest"

nomad version
```

想用 zip的话：`Expand-Archive -Path .\nomad_2.1.1_windows_amd64.zip
-DestinationPath $Dest -Force`，之后同样需要 `Unblock-File`。

##配置

`C:\ProgramData\Nomad\nomad.hcl`：

```hcl
data_dir = "C:/ProgramData/Nomad/data"
bind_addr = "0.0.0.0"
log_level = "INFO"

server {
 enabled = true
 bootstrap_expect = 1
}

client {
 enabled = true

 # raw_exec默认关闭，任务以 agent的权限运行
 options = {
 "driver.raw_exec.enable" = "1"
 }
}
```

HCL里的路径**用正斜杠**。反斜杠多数地方能用，但路径末尾的 `\`会转义结尾引号，
产生莫名其妙的解析错误。

```powershell
New-Item -ItemType Directory -Force -Path 'C:\ProgramData\Nomad\data' | Out-Null
```

##注册为服务

`sc.exe`系统自带，不需要额外软件：

```powershell
sc.exe create Nomad binPath= '"C:\Program Files\Nomad\nomad.exe" agent -config "C:\ProgramData\Nomad"' start= auto
sc.exe description Nomad "Nomad workload orchestrator"
sc.exe start Nomad
```

注意 `binPath=`和 `start=` **后面必须有一个空格**，这是 `sc.exe`的语法要求，
不是笔误。少了它会得到一句毫无帮助的"参数不正确"。

检查服务与日志：

```powershell
Get-Service Nomad
Get-EventLog -LogName Application -Source Nomad -Newest 20
```

配置阶段想在前台跑，便于看输出：

```powershell
& 'C:\Program Files\Nomad\nomad.exe' agent -config 'C:\ProgramData\Nomad'
```

##防火墙

```powershell
New-NetFirewallRule -DisplayName 'Nomad HTTP' -Direction Inbound -Protocol TCP -LocalPort 4646 -Action Allow
New-NetFirewallRule -DisplayName 'Nomad RPC' -Direction Inbound -Protocol TCP -LocalPort 4647 -Action Allow
New-NetFirewallRule -DisplayName 'Nomad Serf' -Direction Inbound -Protocol TCP -LocalPort 4648 -Action Allow
```

未启用 ACL时该 API没有认证。工作站上建议绑定 `127.0.0.1`并不加防火墙规则。

##验证

```powershell
nomad server members
nomad node status
nomad node status -self
```

##第一个任务

```powershell
@'
job "hello" {
 datacenters = ["dc1"]
 type = "batch"

 group "g" {
 task "t" {
 driver = "raw_exec"
 config {
 command = "C:/Windows/System32/cmd.exe"
 args = ["/c", "echo hello from %COMPUTERNAME%"]
 }
 }
 }
}
'@ | Set-Content -Path "$env:TEMP\hello.nomad" -Encoding ascii

nomad job run "$env:TEMP\hello.nomad"
nomad job status hello
```

同一个 job换成对应命令也能在 Linux和 macOS上跑，见
[../../examples/hello-unix.nomad](../../examples/hello-unix.nomad)。更多示例：
[../examples/README.md](../examples/README.md)。

##Windows下的能力边界

|功能 |状态 |
| --- | --- |
| `exec`驱动 |不可用，依赖 Linux专有的隔离机制 |
| `raw_exec` |可用，任务以 agent的账户运行 |
| `java`驱动 |可用，要求 PATH里有 JVM |
| `qemu`驱动 |可用，需要 Windows版 QEMU |
| `docker`驱动 |可用，但只支持 Windows容器；Linux容器需 Docker Desktop |
|桥接网络、CNI |不可用，请用 `network { mode = "host" }` |
| CSI主机路径插件 |需要 Windows构建 |

job里的路径用 Windows形式；`command`应为**绝对路径**，
`raw_exec`不会去 `%PATH%`里搜索命令本身。

##卸载

```powershell
sc.exe stop Nomad
sc.exe delete Nomad
Remove-Item -Recurse -Force 'C:\Program Files\Nomad', 'C:\ProgramData\Nomad'
Remove-NetFirewallRule -DisplayName 'Nomad *'
```

##容易踩的坑

- `sc.exe`的 `binPath=`后面必须有空格，而错误信息不会提示这一点。
-用 `Invoke-WebRequest`下载的二进制首次执行会被拦截，直到运行 `Unblock-File`。
弹窗每个文件只出现一次。
-普通 PowerShell窗口里跑 `nomad agent -dev`，关窗口即进程结束，分配全部消失。
-任务输出写在 `data_dir\alloc`下的文件里，不会进 Windows事件日志；
如需进事件日志，须设 `logs { disabled = true }`并由程序自己写。
