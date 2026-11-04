#!/bin/bash
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
check_dir="$(mktemp -d "${TMPDIR:-/tmp}/PriceReminderAuthChecks.XXXXXX")"
trap 'rm -rf "$check_dir"' EXIT
build_dir="$check_dir/build.noindex"
mkdir -p "$build_dir/AuthChecks.app/Contents/MacOS"
cat > "$build_dir/AuthChecks.app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>test.pricereminder.auth</string><key>CFBundleExecutable</key><string>AuthChecks</string><key>PriceReminderServerURL</key><string>https://example.invalid</string></dict></plist>
PLIST
swiftc -swift-version 6 -emit-library -emit-module -module-name PriceCore "$here/PriceCoreStub.swift" -emit-module-path "$build_dir/PriceCore.swiftmodule" -o "$build_dir/libPriceCore.dylib"
swiftc -swift-version 6 -parse-as-library -I "$build_dir" -L "$build_dir" -lPriceCore "$here/../Shared/ServerConnectionService.swift" "$here/Checks.swift" -o "$build_dir/AuthChecks.app/Contents/MacOS/AuthChecks"
DYLD_LIBRARY_PATH="$build_dir" "$build_dir/AuthChecks.app/Contents/MacOS/AuthChecks"
