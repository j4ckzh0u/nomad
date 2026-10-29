#示例

每个示例都是完整的 job文件，全部在真机上跑过，并标注了适用平台。

|文件 |类型 |适用 |验证了什么 |
| --- | --- | --- | --- |
| [../../examples/hello-unix.nomad](../../examples/hello-unix.nomad) | batch | Linux、macOS |调度器能放置任务，驱动能拉起进程 |
| [../../examples/hello-windows.nomad](../../examples/hello-windows.nomad) | batch | Windows |同上，路径换成 Windows解释器 |
| [../../examples/android-hello.nomad](../../examples/android-hello.nomad) | batch | Android |没有 Docker时 `raw_exec`可用 |
| [../../examples/android-service.nomad](../../examples/android-service.nomad) | service | Android |重启策略与长期运行任务 |
| [../../../demo/android/job-9801.nomad](../../../demo/android/job-9801.nomad) | service | Android |两个 HTTP服务，端口9801和9802 |

##跑一个

```sh
nomad job run docs/examples/hello-unix.nomad
nomad job status hello-unix
nomad job allocs hello-unix
```

batch任务会自己结束。读取它的输出：

```sh
nomad alloc logs <分配ID> t
```

如果任务里设了 `logs { disabled = true }`，就没有日志可读，
这是 Android上的常用设置。此时看退出码：

```sh
nomad job allocs hello-unix -t '{{range .}}{{.ID}} {{.ClientStatus}}{{println}}{{end}}'
```

##双服务测试

这是 Android上最有用的冒烟测试，因为它同时验证了两件事：
两个实例被**独立放置**，且两个都能访问。

```sh
adb push demo/android/httpserver/respond.sh demo/android/httpserver/serve-9801.sh \
 demo/android/httpserver/serve-9802.sh /data/local/tmp/
adb shell chmod 755 /data/local/tmp/respond.sh /data/local/tmp/serve-9801.sh /data/local/tmp/serve-9802.sh

nomad job run demo/android/job-9801.nomad
nomad job run demo/android/job-9802.nomad

curl http://<设备IP>:9801/ # svc-a
curl http://<设备IP>:9802/ # svc-b
```

**空响应或连接重置不代表 job失败。**监听端是 `toybox nc`，
它对连接是串行处理的；先重试一次再排查。同时应该先看 job状态：

```sh
nomad job status
```

##自己写一个

最小可用的 service job：

```hcl
job "myapp" {
 datacenters = ["dc1"]
 type = "service"

 group "g" {
 network {
 mode = "host"
 }

 task "app" {
 driver = "raw_exec"

 config {
 command = "/path/to/application"
 # args = ["--flag"]
 }

 restart {
 attempts = 3
 interval = "5m"
 delay = "5s"
 mode = "fail"
 }
 }
 }
}
```

`raw_exec`以 agent的用户身份运行进程，**没有任何文件系统隔离**。
这正是 Android上需要的（那里没有容器运行时），但也意味着
**job文件属于受信输入**：能提交 job的人，就能以 agent的权限运行任何东西。

如果目标是 shell脚本，请用解释器当命令、把脚本作为参数传入。
直接把 `command`设成脚本路径，即使路径正确也可能报 "could not be found"，
见 [../troubleshooting.md](../troubleshooting.md#把-command指向脚本会失败)。
