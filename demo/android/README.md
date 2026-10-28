# Nomad on Android: example configuration

Minimal single-node configuration used to validate the Android port. See
`contributing/android.md` for the full background.

## Layout on the device

```
/system/bin/nomad or /data/local/tmp/nomad # the binary (see docs for caveats)
/data/local/nomad/android.hcl # this config
/data/local/nomad/data/ # data_dir
/data/local/services/ # host volume for your binaries
```

## Start the agent

```sh
adb push pkg/android_arm64/nomad.so /data/local/tmp/nomad
adb shell chmod +x /data/local/tmp/nomad
adb push demo/android/android.hcl /data/local/nomad/android.hcl
adb shell 'setsid /data/local/tmp/nomad agent -config=/data/local/nomad/android.hcl'
adb forward tcp:4646 tcp:4646
```

`setsid` detaches the agent from the adb shell session so it survives the
shell exiting. Without it the agent receives SIGHUP when the shell is closed.

## Submit a job

```sh
nomad job run demo/android/example.nomad
nomad job status android-example
```

## Notes

- `raw_exec` must be enabled explicitly in the client `options` block.
- Job specs should set `logs { disabled = true }`: log collection spawns the
 `logmon` re-exec helper, and on Android it is simpler to log to logcat.
- The agent does not survive a reboot on its own. Use an init script, a
 foreground service, or your device's own boot hook to start it.
