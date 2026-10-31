#!/bin/bash
set -euo pipefail
root="$(cd "$(dirname "$0")" && pwd)"
# Keep locally built apps outside Spotlight's application index.
build_dir="${TMPDIR:-/tmp}/PriceReminder-build.noindex"
xcodebuild -project "$root/PriceReminder.xcodeproj" -scheme PriceReminderMac \
  -configuration Release -derivedDataPath "$build_dir" CODE_SIGNING_ALLOWED=NO build
echo "Built: $build_dir/Build/Products/Release/PriceReminder.app"
