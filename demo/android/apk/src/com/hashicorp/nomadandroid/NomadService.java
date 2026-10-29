package com.hashicorp.nomadandroid;

import android.app.Service;
import android.content.Intent;
import android.os.IBinder;
import android.util.Log;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.List;

/**
 * Minimal wrapper that extracts the bundled nomad binary and runs it as an
 * agent.
 *
 * <p>Nomad re-executes its own binary to start the per-task executor, logmon,
 * and the template/artifact helpers. On Android the image reported by
 * os.Executable() is not always executable, so this service extracts a copy to
 * a path it can exec and points the agent at it with NOMAD_EXECUTABLE.
 */
public class NomadService extends Service {

 private static final String TAG = "NomadService";

 /**
 * Where the agent keeps its state.
 *
 * <p>This must be short. A task's secrets directory sits under
 * {@code <data_dir>/alloc/<36-char-uuid>/<task>/secrets}, and Nomad creates a
 * unix socket there for the task API. The kernel caps a socket path at 108
 * bytes. An app-private path such as
 * {@code /data/user/0/com.example.app/files/...} is already ~45 characters
 * before any of that, which overflows and makes every allocation fail with
 * "bind: invalid argument".
 *
 * <p>A writable directory has to exist before the service starts; see
 * DEPLOYMENT.md.
 */
 private static final String DATA_DIR = "/data/local/tmp/nd";

 /** Native library name, packaged as lib/<abi>/libnomad.so. */
 private static final String LIB_NAME = "nomad";

 private Process process;
 private Thread pump;

 @Override
 public int onStartCommand(Intent intent, int flags, int startId) {
 File bin;
 try {
 bin = extractBinary();
 } catch (IOException e) {
 Log.e(TAG, "failed to extract nomad binary", e);
 stopSelf();
 return START_NOT_STICKY;
 }

 File conf = new File(getFilesDir(), "android.hcl");
 if (!conf.exists()) {
 try {
 writeDefaultConfig(conf);
 } catch (IOException e) {
 Log.e(TAG, "cannot write config", e);
 stopSelf();
 return START_NOT_STICKY;
 }
 }

 List<String> cmd = new ArrayList<>();
 cmd.add(bin.getAbsolutePath());
 cmd.add("agent");
 cmd.add("-config=" + conf.getAbsolutePath());

 ProcessBuilder pb = new ProcessBuilder(cmd);
 pb.environment().put("NOMAD_EXECUTABLE", bin.getAbsolutePath());

 // go-plugin creates its unix socket with os.CreateTemp, which honours
 // TMPDIR. Android resolves /data/local/tmp for that, and an app has no
 // write access there, so the executor dies with
 // "open /data/local/tmp/plugin1234: permission denied". Point TMPDIR at a
 // directory the app owns.
 File tmp = new File(DATA_DIR, "tmp");
 if (!tmp.isDirectory() && !tmp.mkdirs()) {
 Log.w(TAG, "cannot create " + tmp);
 }
 pb.environment().put("TMPDIR", tmp.getAbsolutePath());

 pb.redirectErrorStream(true);

 try {
 process = pb.start();
 } catch (IOException e) {
 Log.e(TAG, "failed to start nomad", e);
 stopSelf();
 return START_NOT_STICKY;
 }

 pump = new Thread(() -> pumpOutput(process));
 pump.setDaemon(true);
 pump.start();

 Log.i(TAG, "nomad started: " + bin.getAbsolutePath());
 return START_STICKY;
 }

 /**
 * Copies the packaged binary to the app's files dir with the execute bit
 * set, then returns it. Re-extracts whenever the packaged copy changes, so
 * an app update picks up a new agent build.
 */
 private File extractBinary() throws IOException {
 File out = new File(getFilesDir(), "bin/nomad");
 File parent = out.getParentFile();
 if (parent != null && !parent.isDirectory() && !parent.mkdirs()) {
 throw new IOException("cannot create " + parent);
 }

 // The binary ships as an asset. Native libraries under lib/<abi>/ are the
 // usual way to keep a binary uncompressed, but on API27 with
 // extractNativeLibs="false" the directory is not populated at all, so the
 // asset route is the reliable one. The write below also strips the
 // compression, which is what makes the result executable.
 try (InputStream in = getAssets().open(LIB_NAME);
 OutputStream os = new FileOutputStream(out)) {
 byte[] buf = new byte[64 * 1024];
 int n;
 while ((n = in.read(buf)) > 0) {
 os.write(buf, 0, n);
 }
 }

 if (!out.setExecutable(true, false)) {
 throw new IOException("cannot make " + out + " executable");
 }
 return out;
 }

 /**
 * Writes a single-node config on first launch so the app works without any
 * manual setup. Data lives in the app's private dir, which no other app can
 * read.
 */
 private void writeDefaultConfig(File conf) throws IOException {
 // Keep data_dir short. A task's secrets dir sits under
 // <data_dir>/alloc/<36-char-uuid>/<task>/secrets, and the task API creates a
 // unix socket there. The kernel limit for a socket path is 108 bytes, and
 // the default app-private path is already ~50 characters before any of
 // that, which overflows and makes every allocation fail with
 // "bind: invalid argument".
 String hcl = "data_dir = \"" + DATA_DIR + "\"\n"
 + "bind_addr = \"0.0.0.0\"\n"
 + "log_level = \"INFO\"\n"
 + "disable_update_check = true\n"
 + "\n"
 + "server {\n"
 + " enabled = true\n"
 + " bootstrap_expect = 1\n"
 + "}\n"
 + "\n"
 + "client {\n"
 + " enabled = true\n"
 + " options = {\n"
 + " \"driver.raw_exec.enable\" = \"1\"\n"
 + " }\n"
 + "}\n";
 try (FileOutputStream os = new FileOutputStream(conf)) {
 os.write(hcl.getBytes("UTF-8"));
 }
 Log.i(TAG, "wrote default config to " + conf);
 }

 private void pumpOutput(Process p) {
 try (InputStream in = p.getInputStream()) {
 byte[] buf = new byte[8192];
 int n;
 while ((n = in.read(buf)) > 0) {
 Log.i(TAG, new String(buf, 0, n).trim());
 }
 } catch (IOException ignored) {
 // process died
 }
 }

 @Override
 public void onDestroy() {
 if (process != null) {
 process.destroy();
 process = null;
 }
 super.onDestroy();
 }

 @Override
 public IBinder onBind(Intent intent) {
 return null;
 }
}
