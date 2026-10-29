#!/usr/bin/env bash
set -euo pipefail

device="${1:-}"
if [[ -z "$device" ]]; then
  devices="$(adb devices -l)"
  mapfile -t candidates < <(awk 'NR > 1 && / usb:/ { print $1 }' <<< "$devices")
  if (( ${#candidates[@]} == 0 )); then
    mapfile -t candidates < <(awk 'NR > 1 && $1 ~ /^emulator-/ { print $1 }' <<< "$devices")
  fi
  if (( ${#candidates[@]} == 0 )); then
    printf '%s\n' "$devices" >&2
    printf '%s\n' 'ADB chưa thấy điện thoại USB hoặc emulator nào.' 'Trên điện thoại: bật USB debugging, mở khóa màn hình, cắm lại cáp dữ liệu và chấp nhận quyền gỡ lỗi USB, rồi chạy lại make android.' >&2
    exit 1
  fi
  if (( ${#candidates[@]} > 1 )); then
    printf '%s\n' "$devices" >&2
    printf '%s\n' 'Có nhiều thiết bị cùng loại. Chọn một máy bằng make android ANDROID_DEVICE=SERIAL.' >&2
    exit 1
  fi
  device="${candidates[0]}"
fi

if [[ "$(adb -s "$device" get-state)" != device ]]; then
  printf '%s\n' "Thiết bị $device chưa sẵn sàng. Mở khóa máy, bật USB debugging và chấp nhận quyền gỡ lỗi USB." >&2
  exit 1
fi

# Tauri 2 matches the device's display name, not its ADB serial.
if [[ "$device" == emulator-* ]]; then
  device_name="$(adb -s "$device" emu avd name | head -n 1 | tr -d '\r')"
  if [[ -z "$device_name" ]]; then
    device_name="$(adb -s "$device" shell getprop ro.boot.qemu.avd_name | tr -d '\r')"
  fi
else
  device_name="$(adb -s "$device" shell getprop ro.product.model | tr -d '\r')"
fi
if [[ -z "$device_name" ]]; then
  printf 'Không đọc được tên thiết bị %s để chọn trong Tauri.\n' "$device" >&2
  exit 1
fi

printf 'Chạy Anomaly trên %s (%s)\n' "$device_name" "$device"
adb -s "$device" reverse tcp:8080 tcp:8080
adb -s "$device" reverse tcp:8090 tcp:8090
adb -s "$device" reverse tcp:1420 tcp:1420
adb -s "$device" reverse tcp:1421 tcp:1421
mkdir -p "$HOME/.local/bin"
corepack enable --install-directory "$HOME/.local/bin"
export PATH="$PWD/node_modules/.bin:$HOME/.local/bin:$PATH"
exec yarn workspace anomaly-mobile tauri android dev "$device_name" --host 127.0.0.1
