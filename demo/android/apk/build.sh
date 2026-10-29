#!/bin/bash
#
# Build the Nomad Android wrapper APK without Gradle.
#
# AGP versions are tied to specific Gradle releases, and this is meant to be
# buildable from a plain command-line SDK install. aapt2, d8, zipalign and
# apksigner are all that is needed.
#
# Usage:
# ./build.sh [/path/to/nomad-arm64-binary]
#
set -eu

HERE=$(cd "$(dirname "$0")" && pwd)
SDK=${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}
BT_VERSION=${BUILD_TOOLS:-36.1.0}
BT="$SDK/build-tools/$BT_VERSION"
PLATFORM=${PLATFORM:-android-36}
ANDROID_JAR="$SDK/platforms/$PLATFORM/android.jar"

NOMAD_BIN=${1:-}
ABI=${ABI:-arm64-v8a}
OUT="$HERE/build"
KEYSTORE="$HERE/debug.keystore"

if [ -z "$NOMAD_BIN" ]; then
 echo "usage: $0 <nomad-binary>" >&2
 exit 1
fi
for f in "$NOMAD_BIN" "$ANDROID_JAR" "$BT/aapt2" "$BT/d8"; do
 if [ ! -e "$f" ]; then
 echo "missing: $f" >&2
 exit 1
 fi
done

rm -rf "$OUT"
mkdir -p "$OUT/compiled" "$OUT/gen" "$OUT/classes" "$OUT/dex" \
 "$OUT/assets" "$OUT/base"

echo "==> compile resources"
"$BT/aapt2" compile --dir "$HERE/res" -o "$OUT/compiled/res.zip"

echo "==> link resources"
"$BT/aapt2" link \
 -o "$OUT/base/unsigned.apk" \
 -I "$ANDROID_JAR" \
 --manifest "$HERE/AndroidManifest.xml" \
 --java "$OUT/gen" \
 --min-sdk-version 27 \
 --target-sdk-version 27 \
 --auto-add-overlay \
 "$OUT/compiled/res.zip"

echo "==> package the nomad binary as an asset"
# Assets are compressed by aapt2/zip, but the service extracts the bytes to a
# real file and sets the execute bit before running it, so compression is
# fine here.
mkdir -p "$OUT/assets"
cp "$NOMAD_BIN" "$OUT/assets/nomad"
(cd "$OUT" && zip -q -0 -r base/unsigned.apk "assets")

echo "==> compile java"
find "$HERE/src" "$OUT/gen" -name '*.java' > "$OUT/sources.txt"
# JDK11+ rejects -bootclasspath combined with -source/-target, so compile
# against android.jar as a plain classpath entry instead.
javac -source 11 -target 11 \
 -classpath "$ANDROID_JAR" \
 -nowarn \
 -d "$OUT/classes" \
 @"$OUT/sources.txt"

echo "==> dex"
find "$OUT/classes" -name '*.class' > "$OUT/classes.txt"
"$BT/d8" --min-api 27 --output "$OUT/dex" @"$OUT/classes.txt"

echo "==> add classes.dex"
(cd "$OUT/dex" && zip -q -j "$OUT/base/unsigned.apk" classes.dex)

echo "==> zipalign"
"$BT/zipalign" -f -p 4 "$OUT/base/unsigned.apk" "$OUT/base/aligned.apk"

echo "==> sign"
if [ ! -f "$KEYSTORE" ]; then
 keytool -genkeypair -v \
 -keystore "$KEYSTORE" \
 -storepass android -keypass android \
 -alias androiddebugkey \
 -keyalg RSA -keysize 2048 -validity 10000 \
 -dname "CN=Android Debug,O=Android,C=US" >/dev/null 2>&1
fi
"$BT/apksigner" sign \
 --ks "$KEYSTORE" \
 --ks-pass pass:android \
 --key-pass pass:android \
 --ks-key-alias androiddebugkey \
 --out "$OUT/nomad-android.apk" \
 "$OUT/base/aligned.apk"

echo
echo "built: $OUT/nomad-android.apk"
"$BT/apksigner" verify --print-certs "$OUT/nomad-android.apk" | head -n3
