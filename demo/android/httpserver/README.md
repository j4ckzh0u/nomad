# Minimal HTTP service for Nomad on Android

Two tiny services used to check that Nomad places and supervises work on a
device, without pulling in nginx or any other cross-compiled binary.

| File | Serves |
| --- | --- |
| `serve-9801.sh` | port 9801, replies `svc-a` |
| `serve-9802.sh` | port 9802, replies `svc-b` |
| `respond.sh` | answers one connection; invoked by `nc -L` |

## Install

```sh
adb push respond.sh serve-9801.sh serve-9802.sh /data/local/tmp/
adb shell chmod 755 /data/local/tmp/respond.sh /data/local/tmp/serve-9801.sh /data/local/tmp/serve-9802.sh
```

Then run `job-9801.nomad` and `job-9802.nomad` from `demo/android/`.

## Why it is built this way

`toybox nc -L` listens continuously and runs a command per connection with
the socket on stdin and stdout. That is the whole HTTP implementation.

Four details cost real time to get right on the device:

- `-q` must not be used. It closes the socket as soon as stdin hits EOF,
 which happens the moment the response is written, so the client reads
 zero bytes. `-L` manages the lifetime correctly on its own.
- CRLF has to be produced by `printf` at the point of use. Command
 substitution strips trailing newlines, so `CRLF=$(printf '\r\n')`
 yields a bare CR.
- `nc -L` passes no arguments to its command, so the port and name reach
 `respond.sh` through the environment.
- Port and name are baked into each `serve-*.sh` rather than passed on a
 command line, which keeps the launch simple.

## Reliability

`-L` serialises connections. Measured on an RK3399 board, four out of five
sequential requests succeed; the failures are connection resets while the
listener hands off between connections. On an x86_64 redroid image five
out of five succeeded. That is fine for a placement test and not a
production server.
