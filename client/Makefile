.DEFAULT_GOAL := help
.PHONY: help emulator-create emulator-list emulator android desktop

# Run from client/, or use make -C client <target> from the repository root.
help:
	@printf '%s\n' \
	  'Chạy trong client/: make <lệnh> (từ root: make -C client <lệnh>)' \
	  '' \
	  '  emulator-list    Liệt kê máy ảo hiện có' \
	  '  emulator-create  Tạo pixel_35 (API 35) một lần nếu chưa có' \
	  '  emulator         Mở pixel_35 ở terminal 1' \
	  '  android          Chạy client Android ở terminal 2' \
	  '  desktop          Chạy client Linux desktop' \
	  '' \
	  'Các lệnh tự vào Nix shell. Cài dependencies trước khi chạy client.' \
	  'Dừng client cũ bằng Ctrl+C trước khi đổi Android/desktop (chung port 1420).'

emulator-create:
	nix develop .\#android --command avdmanager create avd --name pixel_35 --package 'system-images;android-35;google_apis;x86_64'

emulator-list:
	nix develop .\#android --command emulator -list-avds

emulator:
	nix develop .\#android --command emulator -avd pixel_35

android:
	nix develop .\#android --command bash -c 'set -e; mkdir -p "$$HOME/.local/bin"; corepack enable --install-directory "$$HOME/.local/bin"; export PATH="$$HOME/.local/bin:$$PATH"; exec yarn tauri android dev'

desktop:
	nix develop .\#default --command bash -c 'set -e; mkdir -p "$$HOME/.local/bin"; corepack enable --install-directory "$$HOME/.local/bin"; export PATH="$$HOME/.local/bin:$$PATH"; exec yarn tauri dev'
