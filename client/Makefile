.DEFAULT_GOAL := help
.PHONY: help emulator-create emulator-list emulator-webcams emulator android desktop

CAMERA_FRONT ?= webcam0
ANDROID_DEVICE ?=

ADB_DEVICE_ARGS = $(if $(ANDROID_DEVICE),-s $(ANDROID_DEVICE),-e)
TAURI_DEVICE_ARGS = $(if $(ANDROID_DEVICE),--device $(ANDROID_DEVICE),)

# Run from client/, or use make -C client <target> from the repository root.
help:
	@printf '%s\n' \
	  'Chạy trong client/: make <lệnh> (từ root: make -C client <lệnh>)' \
	  '' \
	  '  emulator-list    Liệt kê máy ảo hiện có' \
	  '  emulator-create  Tạo pixel_33 (API 33) một lần nếu chưa có' \
	  '  emulator-webcams Liệt kê webcam laptop emulator nhận được' \
	  '  emulator         Mở pixel_33 với camera trước dùng webcam0 ở terminal 1' \
	  '  android          Chạy Android (mặc định emulator; ANDROID_DEVICE=SERIAL cho USB)' \
	  '  desktop          Chạy client Linux desktop' \
	  '  release          Build bộ cài app: APP_PLATFORM=linux|windows|android' \
	  '                   Cần VITE_API_ENDPOINT=https://...; xem docs/release-apps.md ở root' \
	  '' \
	  'Đổi camera trước: make emulator CAMERA_FRONT=webcam1 (hoặc emulated nếu không có webcam).' \
	  'Các lệnh tự vào Nix shell. Cài dependencies trước khi chạy client.' \
	  'Dừng client cũ bằng Ctrl+C trước khi đổi Android/desktop (chung port 1420).'

emulator-create:
	nix develop .\#android --command avdmanager create avd --name pixel_33 --package 'system-images;android-33;google_apis;x86_64'

emulator-list:
	nix develop .\#android --command emulator -list-avds

emulator-webcams:
	nix develop .\#android --command emulator -avd pixel_33 -webcam-list

emulator:
	nix develop .\#android --command emulator -avd pixel_33 -camera-front "$(CAMERA_FRONT)"

android:
	nix develop .\#android --command bash -c 'set -e; adb $(ADB_DEVICE_ARGS) reverse tcp:8080 tcp:8080; adb $(ADB_DEVICE_ARGS) reverse tcp:8090 tcp:8090'
	nix develop .\#android --command bash -c 'set -e; mkdir -p "$$HOME/.local/bin"; corepack enable --install-directory "$$HOME/.local/bin"; export PATH="$$PWD/node_modules/.bin:$$HOME/.local/bin:$$PATH"; exec yarn workspace anomaly-mobile tauri android dev $(TAURI_DEVICE_ARGS)'

desktop:
	nix develop .\#default --command bash -c 'set -e; mkdir -p "$$HOME/.local/bin"; corepack enable --install-directory "$$HOME/.local/bin"; export PATH="$$HOME/.local/bin:$$PATH"; exec yarn workspace anomaly-mobile tauri dev'

# Run the same release pipeline from client/ or the monorepo root.
.PHONY: release release-check release-test
release release-check release-test:
	$(MAKE) -C .. $@
