#!/usr/bin/env bash
set -euo pipefail

# package_mac_app.sh <binary> <outdir> <AppName> <BundleID> <IconPath>
BIN_PATH=${1:?binary path required}
OUTDIR=${2:?output dir required}
APP_NAME=${3:?app name required}
BUNDLE_ID=${4:?bundle id required}
ICON_SRC=${5:-}

if [[ ! -f "$BIN_PATH" ]]; then
  echo "Binary not found: $BIN_PATH" >&2
  exit 1
fi

APP_DIR="$OUTDIR/${APP_NAME}.app"
CONTENTS="$APP_DIR/Contents"
MACOS="$CONTENTS/MacOS"
RESOURCES="$CONTENTS/Resources"

mkdir -p "$MACOS" "$RESOURCES"

# Info.plist
cat >"$CONTENTS/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>${APP_NAME}</string>
  <key>CFBundleExecutable</key>
  <string>${APP_NAME}</string>
  <key>CFBundleIdentifier</key>
  <string>${BUNDLE_ID}</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>${APP_NAME}</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0.0</string>
  <key>CFBundleVersion</key>
  <string>1</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.13</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>CFBundleIconFile</key>
  <string>Icon.png</string>
</dict>
</plist>
PLIST

# Copy binary and icon
cp "$BIN_PATH" "$MACOS/${APP_NAME}"
chmod +x "$MACOS/${APP_NAME}"
if [[ -n "$ICON_SRC" && -f "$ICON_SRC" ]]; then
  cp "$ICON_SRC" "$RESOURCES/Icon.png"
fi

echo "Packaged: $APP_DIR"

