#故障排查

本页每一条都是在真机上实际撞到的问题。每条给出**原始报错**、**根因**和**解决办法**。

Android的问题放在最前面，因为它最不常见。凡是报错信息**在误导人**的地方，
都会明确指出来。

##Android

###找不到 nobody用户

```
failed to setup alloc: pre-run hook "alloc_dir" failed:
Unable to find nobody user: user: Lookup not implemented on android
```

**根因。** Android没有用户数据库。不存在 `/etc/passwd`，uid来自 bionic的
`AID_*`常量表，而 Go的 `os/user`是个桩实现，只能解析当前用户。
Nomad在准备每个分配目录时都会解析 `nobody`并把失败当作致命错误，
结果**一个任务都起不来**。

**解决。**本分支把"平台没有用户数据库"和"这个用户不存在"区分开，改为降级：
目录保持 agent所有。实际上它本来就会是 agent所有，因为目录已经被置为 0777。

如果你用的是旧版本，Android上不会有任何任务能跑。**这是第一个该检查的点**。

**确认方式。** `nomad job status`应该显示 `running`，而不是在
`Task Setup`阶段 `failed`。

###辅助进程起不来

```
failed to create executor: error creating rpc client for executor plugin:
Unrecognized remote plugin message:
Failed to read any lines from plugin's stdout
This usually means
 the plugin was not compiled for this architecture,
 the plugin is missing dynamic-link libraries necessary to run,
 the plugin is not executable by this process due to file permissions, or
 the plugin failed to negotiate the initial go-plugin protocol handshake
```

在 Android上，这段话**每一句都是错的**。架构没问题，权限也没问题。
有两种完全不同的原因都会产生这条报错。

**原因A：运行中的镜像不可执行。** Nomad会重新执行自身来启动 executor和
logmon。在 APK内部，镜像是从 APK映射出来的，没有可用的路径。

**解法A。**告诉它真实副本在哪：

```hcl
executable_path = "/data/local/tmp/nomad"
```

或设置环境变量 `NOMAD_EXECUTABLE`。覆盖生效时 agent会打一条警告，
说明辅助进程使用的二进制与运行镜像不同。

**原因B：`TMPDIR`不可写。** go-plugin用 `os.CreateTemp`创建 socket，
而 Android把 `TMPDIR`解析为 `/data/local/tmp`，应用在那里没有写权限。

**解法B。**启动前把 `TMPDIR`指向 agent自己的目录：

```sh
TMPDIR=/data/user/0/<包名>/files/tmp nomad agent -config ...
```

**怎么区分A和B。**真实原因只出现在 executor自己的日志里：

```sh
adb shell cat <data_dir>/alloc/<分配ID>/<任务名>/executor.out
```

出现 `plugin init error: open /data/local/tmp/plugin 123: permission denied`
这类内容就是原因B。如果文件存在但为空，说明进程根本没起来，是原因A。

###创建任务 socket时报 `bind: invalid argument`

```
error creating task api socket: listen unix .../secrets/api.sock: bind: invalid argument
```

**根因。**内核把 unix socket路径限制在 108字节。任务的密钥目录是
`<data_dir>/alloc/<36位uuid>/<任务名>/secrets`，`data_dir`一长就溢出。
应用私有路径如 `/data/user/0/com.example.app/files`，在拼接之前就已经
大约 45个字符了。

**解决。**用短的 `data_dir`，例如 `/data/local/tmp/nd`。
有两个目录必须存在且可写：

```sh
adb shell mkdir -p /data/local/tmp/nd /data/local/tmp/alloc_mounts
adb shell chmod 777 /data/local/tmp/nd /data/local/tmp/alloc_mounts
```

`alloc_mounts`是客户端挂载目录下的固定路径，无法改位置，所以要做同样处理。

**部署前先算一下长度：**

```sh
python3 -c "print(len('/data/local/tmp/nd/alloc/' + '0'*36 + '/t/secrets/api.sock'))"
```

超过108，第一个分配就会失败。

### `failed to detect memset`

```
failed to initialize process manager: failed to detect memset:
open /sys/fs/cgroup/cpuset/cpuset.mems: no such file or directory
```

**根因。**镜像把 cgroup2挂载**叠加**在 cgroup v1层级之上。
检测代码在读到 `/proc/self/mountinfo`里第一条 `/sys/fs/cgroup`时就返回，
看到的是外层 tmpfs，判定为 cgroups v1，随后去读一个已被 cgroup2遮蔽的路径。

在 redroid Android12 x86_64上复现。纯 v1的镜像（例如 RK3399上的
Android8.1）不会出现。

**解决。**现在会扫完整个文件并以该路径的**最后一条**为准，因为那才是实际
可见的挂载。验证方式是客户端报告 cgroups v2并创建 `/sys/fs/cgroup/nomad.slice`。

**在设备上确认：**

```sh
adb shell cat /proc/self/mountinfo | grep /sys/fs/cgroup
```

如果 `/sys/fs/cgroup`出现多次，就是这个原因。

### init从不启动服务

重启后 agent没起来，`getprop init.svc.nomad`是空的，而 `dmesg`里有：

```
init: service nomad does not have a SELinux domain defined
```

**根因。** `/system/etc/init`下的 service**必须**声明 `seclabel`。
没有它，init会把文件解析完、把 action执行完，然后拒绝启动服务。
默认日志级别下没有其他提示，所以看起来像文件被忽略了。

**解决。**加上这一行，用 userdebug版本上可用的宽容域：

```
service nomad /system/bin/nomad agent -config=/data/local/nomad/android.hcl
 class late_start
 user root
 group root system
 seclabel u:r:su:s0
```

生产设备需要自己的域和 sepolicy；`su`只存在于开发版本上。

两处相关的错误，而且都是静默的：

- `on init`块必须出现在 service**之前**，因为 init是按文件顺序求值的。
- `mkdir`的权限是普通八进制：`777`，不是 `0777`。

**确认方式：** `adb shell dmesg | grep nomad`，应能看到
`Parsing file /system/etc/init/nomad.rc`以及后面的 `processing action`。

### tmpfs挂载越积越多

硬重启之后，旧的分配目录仍保留着 `secrets`和 `private`的 tmpfs挂载：

```sh
adb shell grep -c 'nomad/data/alloc' /proc/mounts
```

**根因。** `AllocDir.Destroy`正常情况下会卸载它们。但 `SIGKILL`或断电会跳过，
内核一直保留挂载到重启；之后新的 agent看到密钥目录非空（因为存在
`.nomad-mount`标记文件）就跳过重新挂载，于是这些挂载永远不会被复用或释放。

**解决。**本分支在每次分配时清扫一次：分配目录下凡不是当前正在构建的分配，
就卸载并重建。仍然 busy的挂载会被跳过。

**旧版本可手工清理：**

```sh
adb shell 'grep "nomad/data/alloc" /proc/mounts | while read -r l; do umount $(echo "$l" | cut -d" " -f2); done'
```

### `stop`之后任务还在跑

停掉 agent后 `ps`里仍有任务进程，端口也还被占用。

**根因。**这不是缺陷。客户端在 `SIGTERM`时**故意不动分配**，
避免重启打断业务，executor会被 init接管。`allocRunner.Shutdown`的注释里
写得很清楚："Tasks are unaffected and may be restored"。

**解决。**先通过 API停任务：

```sh
nomad job stop <任务名>
```

`demo/android`里的启动脚本会在给 agent发信号之前做这件事。
只 kill agent、或者 `pkill nomad`，都不会停掉负载。

### `logmon`不稳定

任务日志收集的方式是把 agent二进制重新执行为 `logmon`。
在 Android上这层转发很脆弱。

**解决。**按任务关闭它，让应用自己输出日志：

```hcl
task "svc" {
 driver = "raw_exec"
 logs {
 disabled = true
 }
}
```

应用可以写 `logcat`，然后用 `adb logcat -s <TAG>`查看。

###把 `command`指向脚本会失败

```
failed to launch command with executor: rpc error: code = Unknown desc =
binary "/data/local/tmp/serve-9801.sh" could not be found
```

路径存在、权限 0755、属主是 root，手工执行完全正常。

**解决。**用解释器当命令，把脚本作为参数传入：

```hcl
config {
 command = "/system/bin/sh"
 args = ["/data/local/tmp/serve-9801.sh"]
}
```

这种形式稳定可用。直接指向脚本失败的确切根因**没有定位**；
如果你在别的布局下也遇到，应当作 bug上报，而不是当成预期行为。

##全平台通用

###节点是 ready但任务可行性检查失败

```
no nodes available to place the task group
```

**根因。**节点在驱动完成指纹采集之前就会变成 `ready`。
在这个窗口内提交任务会因缺少驱动而失败。

**解决。**等几秒，或先看驱动列表：

```sh
nomad node status -self
```

### `job stop`之后再 `job run`看到旧分配

旧的分配在 stop之后仍显示为 `complete`。这是垃圾回收的延迟，
不是任务被复活了。

**解决。**不需要处理。写脚本时按状态过滤：

```sh
nomad job allocs <任务名> -t '{{range .}}{{if eq .ClientStatus "running"}}{{.ID}}{{end}}{{end}}'
```

### macOS上二进制无法运行

```
Bad CPU type in executable
```

**根因。**下错了包。`darwin_amd64`在没有 Rosetta的 Apple Silicon上跑不了，
`darwin_arm64`在 Intel上完全跑不了。

**解决。**确认机器架构后重新下载：

```sh
uname -m
```

### macOS拒绝打开二进制

```
"nomad" cannot be opened because the developer cannot be verified
```

**根因。**压缩包未签名、未公证，macOS会隔离它。

**解决。**

```sh
xattr -d com.apple.quarantine /usr/local/bin/nomad
```

### Windows首次运行被拦截

**根因。**用 `Invoke-WebRequest`下载的文件带"网络标记"。

**解决。**对可执行文件执行一次 `Unblock-File`。

##构建相关

### `no required module provides package github.com/hashicorp/go-set`

```
nomad/structs/structs.generated.go:12:2: no required module provides package
github.com/hashicorp/go-set
```

**根因。** `structs.generated.go`受 `codegen_generated`构建标签保护，
内部 import了 `github.com/hashicorp/go-set`，而 `go.mod`里只有 `go-set/v3`。
**任何设置该标签的构建都会失败**，包括本树上默认的 `make`目标。

**解决。**不要设这个标签：

```sh
NOMAD_NO_UI=1 make GO_TAGS=release pkg/linux_amd64/nomad
```

发布工作流正是出于这个原因，按目标**覆盖** `GO_TAGS`而不是追加。

### `android/amd64 requires external (cgo) linking`

```
android/amd64 requires external (cgo) linking, but cgo is not enabled
```

**根因。** Go工具链不允许在没有 cgo的情况下构建 android/amd64和 android/386，
因为产物必须链接 bionic而不是 glibc。

**解决。**提供 NDK编译器：

```sh
NDK=$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$(uname | tr 'A-Z' 'a-z')-x86_64
NOMAD_NO_UI=1 make pkg/android_amd64/nomad \
 CC=$NDK/bin/x86_64-linux-android31-clang
```

arm64和 arm不需要这些，纯 Go即可。

##上报新问题

请附上：

- `nomad version`的输出
- `adb shell getprop ro.build.version.sdk`、`ro.product.cpu.abi`、`getenforce`
- `nomad node status -self`
- executor日志：`<data_dir>/alloc/<分配ID>/<任务名>/executor.out`

最后一个文件保存了驱动到任务之间任何失败的**真实原因**，
Android上的失败绝大多数就发生在这一段。
