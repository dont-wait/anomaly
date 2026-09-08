.PHONY: emulator android desktop

# Run from client/, or use make -C client <target> from the repository root.
emulator:
	nix develop .\#android --command emulator -avd pixel_35

android:
	nix develop .\#android --command bash -c 'set -e; mkdir -p "$$HOME/.local/bin"; corepack enable --install-directory "$$HOME/.local/bin"; export PATH="$$HOME/.local/bin:$$PATH"; exec yarn tauri android dev'

desktop:
	nix develop .\#default --command bash -c 'set -e; mkdir -p "$$HOME/.local/bin"; corepack enable --install-directory "$$HOME/.local/bin"; export PATH="$$HOME/.local/bin:$$PATH"; exec yarn tauri dev'
