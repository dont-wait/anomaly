{
  description = "Tauri client development environments";

  nixConfig = {
    extra-substituters = [ "https://look.cachix.org" ];
    extra-trusted-public-keys = [ "look.cachix.org-1:8elPCeSVBzlDZXqIRKBK9GyLIK/Hoe1xiWZF0ir7uX4=" ];
  };

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay.url = "github:oxalica/rust-overlay";
    rust-overlay.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      nixpkgs,
      flake-utils,
      rust-overlay,
      ...
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
    in
    flake-utils.lib.eachSystem systems (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ rust-overlay.overlays.default ];
        };

        windowsTarget = "x86_64-pc-windows-gnu";

        rustToolchain = pkgs.rust-bin.stable.latest.default.override {
          extensions = [
            "rust-src"
            "rustfmt"
            "clippy"
          ];
          targets = [ windowsTarget ];
        };

        linuxLibraries = with pkgs; [
          alsa-lib
          cairo
          dbus
          gdk-pixbuf
          glib
          gtk3
          harfbuzz
          libayatana-appindicator
          librsvg
          libsoup_3
          openssl
          pango
          webkitgtk_4_1
        ];

        linuxLibraryPath = pkgs.lib.makeLibraryPath linuxLibraries;
        mingwPrefix = pkgs.pkgsCross.mingwW64.stdenv.cc.targetPrefix;
      in
      {
        formatter = pkgs.nixfmt;

        devShells.default = pkgs.mkShell {
          nativeBuildInputs = with pkgs; [
            cargo-tauri
            nodejs_22
            pkg-config
            rust-analyzer-unwrapped
            rustToolchain
            yarn
          ];

          buildInputs = linuxLibraries;

          shellHook = ''
            export LD_LIBRARY_PATH="${linuxLibraryPath}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
            export GSETTINGS_SCHEMA_DIR="${pkgs.gtk3}/share/gsettings-schemas/${pkgs.gtk3.name}/glib-2.0/schemas''${GSETTINGS_SCHEMA_DIR:+:$GSETTINGS_SCHEMA_DIR}"
            export RUST_SRC_PATH="${rustToolchain}/lib/rustlib/src/rust/library"
          '';
        };

        devShells.windows = pkgs.mkShell {
          nativeBuildInputs = with pkgs; [
            cargo-tauri
            nodejs_22
            nsis
            pkg-config
            pkgsCross.mingwW64.stdenv.cc
            rustToolchain
            yarn
          ];

          shellHook = ''
            export CARGO_BUILD_TARGET="${windowsTarget}"
            export CARGO_TARGET_X86_64_PC_WINDOWS_GNU_LINKER="${mingwPrefix}cc"
            export CC_x86_64_pc_windows_gnu="${mingwPrefix}cc"
            export CXX_x86_64_pc_windows_gnu="${mingwPrefix}c++"
            export AR_x86_64_pc_windows_gnu="${mingwPrefix}ar"
            export PKG_CONFIG_ALLOW_CROSS=1
            export RUST_SRC_PATH="${rustToolchain}/lib/rustlib/src/rust/library"
          '';
        };
      }
    );
}
