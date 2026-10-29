# Installing Nomad on Windows

Windows is well supported as a Nomad client. Servers also run here, which is
convenient for a lab; production clusters usually keep servers on Linux.

Verified on Windows 10 and 11, and Windows Server 2022.

## Requirements

| Item | Why |
| --- | --- |
| Windows 10 or Server 2016 or later | The binary is built for x86_64 only |
| Administrator PowerShell | Installing a service and opening firewall ports |
| Ports 4646, 4647, 4648 free | Cluster communication |

Run the commands below in an **Administrator** PowerShell. `Start-Process
-Verb RunAs` is not enough for the service registration step.

## Install

```powershell
$Version = '2.1.1'
$Url = "https://github.com/j4ckzh0u/nomad/releases/download/v$Version/nomad_${Version}_windows_amd64.exe"
$Dest = 'C:\Program Files\Nomad'

New-Item -ItemType Directory -Force -Path $Dest | Out-Null
Invoke-WebRequest -Uri $Url -OutFile "$Dest\nomad.exe"

# Files downloaded from the internet carry a mark-of-the-web that makes
# Windows prompt on first run. Removing it once keeps the service usable.
Unblock-File "$Dest\nomad.exe"

# The install directory must be on PATH for the CLI to resolve.
[Environment]::SetEnvironmentVariable(
  'Path',
  [Environment]::GetEnvironmentVariable('Path', 'Machine') + ";$Dest",
  'Machine'
)
$env:Path += ";$Dest"

nomad version
```

If you prefer a zip, use `Expand-Archive -Path .\nomad_2.1.1_windows_amd64.zip
-DestinationPath $Dest -Force` and still run `Unblock-File`.

## Configuration

`C:\ProgramData\Nomad\nomad.hcl`:

```hcl
data_dir  = "C:/ProgramData/Nomad/data"
bind_addr = "0.0.0.0"
log_level = "INFO"

server {
  enabled          = true
  bootstrap_expect = 1
}

client {
  enabled = true

  # raw_exec ships disabled and runs tasks with the agent's privileges.
  options = {
    "driver.raw_exec.enable" = "1"
  }
}
```

Use forward slashes in HCL paths. Backslashes work in most places, but a
trailing `\` escapes the closing quote and produces a confusing parse error.

```powershell
New-Item -ItemType Directory -Force -Path 'C:\ProgramData\Nomad\data' | Out-Null
```

## Run it as a service

`sc.exe` is built in and does not need extra software:

```powershell
sc.exe create Nomad binPath= '"C:\Program Files\Nomad\nomad.exe" agent -config "C:\ProgramData\Nomad"' start= auto
sc.exe description Nomad "Nomad workload orchestrator"
sc.exe start Nomad
```

The space after `binPath=` and `start=` is required by `sc.exe`; it is not a
typo. Without it the command fails with an unhelpful "The parameter is
incorrect".

Check the service and the logs:

```powershell
Get-Service Nomad
Get-EventLog -LogName Application -Source Nomad -Newest 20
```

To run it in the foreground instead, which is easier while configuring:

```powershell
& 'C:\Program Files\Nomad\nomad.exe' agent -config 'C:\ProgramData\Nomad'
```

## Firewall

```powershell
New-NetFirewallRule -DisplayName 'Nomad HTTP' -Direction Inbound -Protocol TCP -LocalPort 4646 -Action Allow
New-NetFirewallRule -DisplayName 'Nomad RPC'  -Direction Inbound -Protocol TCP -LocalPort 4647 -Action Allow
New-NetFirewallRule -DisplayName 'Nomad Serf' -Direction Inbound -Protocol TCP -LocalPort 4648 -Action Allow
```

The HTTP API is unauthenticated unless ACLs are configured. On a workstation,
prefer binding to `127.0.0.1` and leaving the firewall rule out.

## Verify

```powershell
nomad server members
nomad node status
nomad node status -self
```

## First job

```powershell
@'
job "hello" {
  datacenters = ["dc1"]
  type        = "batch"

  group "g" {
    task "t" {
      driver = "raw_exec"
      config {
        command = "C:/Windows/System32/cmd.exe"
        args    = ["/c", "echo hello from %COMPUTERNAME%"]
      }
    }
  }
}
'@ | Set-Content -Path "$env:TEMP\hello.nomad" -Encoding ascii

nomad job run "$env:TEMP\hello.nomad"
nomad job status hello
```

This job also works on Linux and macOS after swapping the command; see
[../examples/hello-unix.nomad](../examples/hello-unix.nomad). More examples:
[../examples/README.md](../examples/README.md).

## Windows specifics

| Feature | Status |
| --- | --- |
| `exec` driver | Unavailable. Linux-only isolation primitives |
| `raw_exec` | Works; tasks run as the agent's account |
| `java` driver | Works if a JVM is on PATH |
| `qemu` driver | Works with a Windows build of QEMU |
| `docker` driver | Works with Windows containers; Linux containers need Docker Desktop |
| Bridge networking, CNI | Unavailable. Use `network { mode = "host" }` |
| CSI host-path plugins | Windows builds required |

Paths inside a job use the Windows form, and `command` should be an absolute
path: `raw_exec` does not consult `%PATH%` for the command itself.

## Uninstall

```powershell
sc.exe stop Nomad
sc.exe delete Nomad
Remove-Item -Recurse -Force 'C:\Program Files\Nomad', 'C:\ProgramData\Nomad'
Remove-NetFirewallRule -DisplayName 'Nomad *'
```

## Things that surprise people

- `sc.exe` needs a space after `binPath=`. The error message does not mention it.
- A binary downloaded with `Invoke-WebRequest` is blocked on first execution
  until `Unblock-File` runs. The dialog appears once per file.
- `nomad agent -dev` in a normal PowerShell stops when the window closes.
  Allocations die with it.
- Task output is written to files under `data_dir\alloc`, not the Windows event
  log, unless `logs { disabled = true }` and the task writes to the event log
  itself.