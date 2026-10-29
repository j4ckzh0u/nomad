package com.hashicorp.nomadandroid;

import android.app.Activity;
import android.content.Intent;
import android.os.Bundle;
import android.widget.TextView;

/** Launcher activity: starts the agent service and shows where things live. */
public class StartActivity extends Activity {

 @Override
 protected void onCreate(Bundle savedInstanceState) {
 super.onCreate(savedInstanceState);

 startService(new Intent(this, NomadService.class));

 TextView tv = new TextView(this);
 tv.setText("Nomad agent starting.\n\n"
 + "HTTP API: http://127.0.0.1:4646\n"
 + "Data dir: " + getFilesDir() + "\n\n"
 + "Logs: adb logcat -s NomadService");
 tv.setPadding(48,48,48,48);
 setContentView(tv);
 }
}
