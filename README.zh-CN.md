Nomad（Android支持分支）
===

[English](README.md)

本仓库是 [Nomad](https://github.com/hashicorp/nomad)的一个分支，增加了**可用的
Android目标**：客户端可以跑在 Android设备上，用 `raw_exec`驱动直接托管二进制
程序，**不需要 Docker，也不需要任何容器运行时**。

Nomad本身是一个简单而灵活的工作负载编排器，可以部署和管理容器
（[docker](https://developer.hashicorp.com/nomad/docs/deploy/task-driver/docker)、
[podman](https://developer.hashicorp.com/nomad/plugins/drivers/podman)）、
非容器化应用（[executable](https://developer.hashicorp.com/nomad/docs/deploy/task-driver/exec)、
[Java](https://developer.hashicorp.com/nomad/docs/deploy/task-driver/java)）以及
虚拟机（[qemu](https://developer.hashicorp.com/nomad/docs/deploy/task-driver/qemu)），
既能跑在本地机房，也能跨云规模化运行。

上游官方支持 Linux、Windows和 macOS。本分支在此之外增加 Android，
并在真实设备上验证过。

##这个分支解决的核心问题

Nomad**不是**直接 fork你的程序。它为了启动每个任务的执行器（executor）
和日志收集器（logmon），**必须重新执行自己的二进制**。而 Android上：

- `/etc/passwd`不存在，uid来自 bionic的 `AID_*`表，按用户名解析必然失败；
-运行中的二进制常常**无法再次执行**（例如从 APK内映射出来的镜像）。

两件事任何一件处理不好，**所有任务都会失败**，而且报错信息极其误导人。
本分支的处理方式是：用户查不到就降级而不是中止；并提供
`executable_path` / `NOMAD_EXECUTABLE`让辅助进程指向一份可执行的副本。

##下载

每个版本发布7个产物，请按**将要运行 Nomad的机器**选择：

|平台 |文件 |
| --- | --- |
| Linux | `nomad_<版本>_linux_amd64.zip`、`nomad_<版本>_linux_arm64.zip` |
| macOS | `nomad_<版本>_darwin_amd64.zip`、`nomad_<版本>_darwin_arm64.zip` |
| Windows | `nomad_<版本>_windows_amd64.exe` |
| Android | `nomad_<版本>_android_arm64.so`、`nomad_<版本>_android_amd64.so` |

Android产物带 `.so`后缀，因为这是 Android允许从 APK内执行的唯一形式；
解压后它是普通可执行文件，按 `nomad`使用。

发布页面：<https://github.com/j4ckzh0u/nomad/releases>

##文档

|内容 |中文 |英文 |
| --- | --- | --- |
|总览与平台对照 | [docs/zh-CN/README.md](docs/zh-CN/README.md) | [docs/README.md](docs/README.md) |
| Linux安装 | [docs/zh-CN/install/linux.md](docs/zh-CN/install/linux.md) | [docs/install/linux.md](docs/install/linux.md) |
| macOS安装 | [docs/zh-CN/install/macos.md](docs/zh-CN/install/macos.md) | [docs/install/macos.md](docs/install/macos.md) |
| Windows安装 | [docs/zh-CN/install/windows.md](docs/zh-CN/install/windows.md) | [docs/install/windows.md](docs/install/windows.md) |
| Android安装 | [docs/zh-CN/install/android.md](docs/zh-CN/install/android.md) | [docs/install/android.md](docs/install/android.md) |
|故障排查 | [docs/zh-CN/troubleshooting.md](docs/zh-CN/troubleshooting.md) | [docs/troubleshooting.md](docs/troubleshooting.md) |
|示例 | [docs/zh-CN/examples/README.md](docs/zh-CN/examples/README.md) | [docs/examples/README.md](docs/examples/README.md) |
| Android移植说明 | — | [contributing/android.md](contributing/android.md) |

**遇到 Android上的问题，请先读故障排查那一页。**里面记录了 11个真机问题，
每条都有原始报错、根因和解决办法，其中多条报错信息本身在误导人。

##快速自检

```sh
./docs/smoke-test.sh /path/to/nomad
```

脚本会启动一个 `-dev`模式的 agent、等待节点就绪、跑一个任务并报告结果。
退出码为0表示链路是通的。

或者手动三步：

```sh
nomad version
nomad agent -dev &
nomad node status
```

##Android能力边界

|功能 |状态 |
| --- | --- |
| `raw_exec` |可用，任务以 agent的 uid运行 |
| `exec`驱动 |arm64未构建（需要 cgo与 libcontainer） |
| `docker`驱动 |未注册，Android没有守护进程 |
|桥接网络、CNI |不可用，用 `network { mode = "host" }` |
| cgroup资源限制 |实际不生效，数值仅参与调度决策 |
|任务日志收集 |能用但脆弱，建议 `logs { disabled = true }` |

##构建

Linux、macOS、Windows和 Android(arm64)都是纯 Go构建：

```sh
NOMAD_NO_UI=1 make GO_TAGS=release pkg/linux_amd64/nomad
```

android/amd64和 android/386**必须用 cgo加 NDK**，
因为 Go工具链要求外部链接、产物需链接 bionic：

```sh
NDK=$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$(uname | tr 'A-Z' 'a-z')-x86_64
NOMAD_NO_UI=1 make pkg/android_amd64/nomad \
 CC=$NDK/bin/x86_64-linux-android31-clang
```

**注意不要设 `codegen_generated`标签**：本树里 `structs.generated.go`
受该标签保护且引用了 `go.mod`中不存在的 `github.com/hashicorp/go-set`，
任何带此标签的构建都会失败。发布工作流因此按目标覆盖 `GO_TAGS`。

##上游资料

* [Nomad文档](https://developer.hashicorp.com/nomad/docs)
* [CLI文档](https://developer.hashicorp.com/nomad/commands)
* [API文档](https://developer.hashicorp.com/nomad/api-docs)
* [插件文档](https://developer.hashicorp.com/nomad/plugins)
* [教程](https://developer.hashicorp.com/nomad/tutorials)
*论坛：[Discuss](https://discuss.hashicorp.com/c/nomad)

##许可

与上游一致，Business Source License 1.1（BUSL-1.1），详见 [LICENSE](LICENSE)。
