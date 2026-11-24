#!/bin/bash
# CI-only packaging. Never change the user's default keychain or existing ACLs.
set -euo pipefail
app="${1:?app path required}"
version="${2:?version required}"
package="PriceReminder-macOS-${version}.dmg"
signing_dir=""
cleanup() {
  if [[ -n "$signing_dir" ]]; then
    security delete-keychain "$signing_dir/signing.keychain-db" >/dev/null 2>&1 || true
    rm -rf "$signing_dir"
  fi
}
trap cleanup EXIT

if [[ -n "${MACOS_CERTIFICATE_P12_BASE64:-}" ]]; then
  : "${MACOS_CERTIFICATE_PASSWORD:?missing certificate password}"
  : "${MACOS_SIGNING_IDENTITY:?missing Developer ID Application identity}"
  : "${MACOS_NOTARY_KEY_P8:?missing App Store Connect API private key}"
  : "${MACOS_NOTARY_KEY_ID:?missing API key ID}"
  : "${MACOS_NOTARY_ISSUER_ID:?missing API issuer ID}"
  [[ "$MACOS_SIGNING_IDENTITY" == "Developer ID Application: "* ]] || {
    echo 'Expected a Developer ID Application identity' >&2; exit 1;
  }
  signing_dir="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/PriceReminderSigning.XXXXXX")"
  keychain_password="$(openssl rand -hex 32)"
  printf '%s' "$MACOS_CERTIFICATE_P12_BASE64" | base64 --decode > "$signing_dir/certificate.p12"
  printf '%s' "$MACOS_NOTARY_KEY_P8" > "$signing_dir/notary.p8"
  chmod 600 "$signing_dir/"*
  security create-keychain -p "$keychain_password" "$signing_dir/signing.keychain-db"
  security set-keychain-settings -lut 21600 "$signing_dir/signing.keychain-db"
  security unlock-keychain -p "$keychain_password" "$signing_dir/signing.keychain-db"
  security import "$signing_dir/certificate.p12" -k "$signing_dir/signing.keychain-db" \
    -P "$MACOS_CERTIFICATE_PASSWORD" -T /usr/bin/codesign >/dev/null
  security set-key-partition-list -S apple-tool:,apple: -s -k "$keychain_password" \
    "$signing_dir/signing.keychain-db" >/dev/null
  codesign --force --deep --options runtime --timestamp \
    --entitlements apple/macOS/PriceReminderMac.entitlements \
    --keychain "$signing_dir/signing.keychain-db" --sign "$MACOS_SIGNING_IDENTITY" "$app"
else
  # Preserve unsigned releases until the owner supplies Developer ID credentials.
  # A partially configured signing setup must never silently downgrade.
  for value in "${MACOS_SIGNING_IDENTITY:-}" "${MACOS_CERTIFICATE_PASSWORD:-}" \
    "${MACOS_NOTARY_KEY_P8:-}" "${MACOS_NOTARY_KEY_ID:-}" "${MACOS_NOTARY_ISSUER_ID:-}"; do
    [[ -z "$value" ]] || { echo 'Incomplete macOS signing secrets' >&2; exit 1; }
  done
  echo '::warning::Developer ID not configured: ad-hoc build; upgrades may require Keychain authorization again.'
  codesign --force --deep --sign - "$app"
fi
codesign --verify --deep --strict --verbose=2 "$app"
mkdir -p dist/dmg
# ditto preserves bundle metadata and extended attributes.
ditto "$app" dist/dmg/PriceReminder.app
ln -s /Applications dist/dmg/Applications
hdiutil create -volname PriceReminder -srcfolder dist/dmg -ov -format UDZO "$package"
if [[ -n "$signing_dir" ]]; then
  codesign --timestamp --keychain "$signing_dir/signing.keychain-db" --sign "$MACOS_SIGNING_IDENTITY" "$package"
  xcrun notarytool submit "$package" --key "$signing_dir/notary.p8" \
    --key-id "$MACOS_NOTARY_KEY_ID" --issuer "$MACOS_NOTARY_ISSUER_ID" --wait --timeout 20m
  xcrun stapler staple "$package"
  xcrun stapler validate "$package"
  spctl --assess --type open --context context:primary-signature --verbose=2 "$package"
fi
