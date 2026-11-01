.DEFAULT_GOAL := help
.PHONY: help emulator-create emulator-list emulator-webcams emulator android desktop

CAMERA_FRONT ?= webcam0
ANDROID_DEVICE ?=

# Run from client/, or use make -C client <target> from the repository root.
help:
	@printf '%s\n' \
	  'Chạy trong client/: make <lệnh> (từ root: make -C client <lệnh>)' \
	  '' \
	  '  emulator-list    Liệt kê máy ảo hiện có' \
	  '  emulator-create  Tạo pixel_33 (API 33) một lần nếu chưa có' \
	  '  emulator-webcams Liệt kê webcam laptop emulator nhận được' \
	  '  emulator         Mở pixel_33 với camera trước dùng webcam0 ở terminal 1' \
	  '  android          Chạy Android (ưu tiên USB; ANDROID_DEVICE=SERIAL để chọn máy)' \
	  '  desktop          Chạy app admin trên Linux desktop (port 1422)' \
	  '' \
	  'Đổi camera trước: make emulator CAMERA_FRONT=webcam1 (hoặc emulated nếu không có webcam).' \
	  'Các lệnh tự vào Nix shell. Cài dependencies trước khi chạy client.' \
	  'Port cố định: Android 1420, desktop 1422. Dừng yarn dev:desktop trước khi chạy make desktop.'

emulator-create:
	nix develop .\#android --command avdmanager create avd --name pixel_33 --package 'system-images;android-33;google_apis;x86_64'

emulator-list:
	nix develop .\#android --command emulator -list-avds

emulator-webcams:
	nix develop .\#android --command emulator -avd pixel_33 -webcam-list

emulator:
	nix develop .\#android --command emulator -avd pixel_33 -camera-front "$(CAMERA_FRONT)"

android:
	nix develop .\#android --command bash android-dev.sh "$(ANDROID_DEVICE)"

desktop:
	nix develop .\#default --command bash -c 'set -e; mkdir -p "$$HOME/.local/bin"; corepack enable --install-directory "$$HOME/.local/bin"; export PATH="$$PWD/node_modules/.bin:$$HOME/.local/bin:$$PATH"; exec yarn workspace anomaly-desktop tauri dev'
