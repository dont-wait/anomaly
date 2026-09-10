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

        androidRustToolchain = pkgs.rust-bin.stable.latest.default.override {
          extensions = [
            "rust-src"
            "rustfmt"
            "clippy"
          ];
          targets = [
            "aarch64-linux-android"
            "armv7-linux-androideabi"
            "i686-linux-android"
            "x86_64-linux-android"
          ];
        };

        androidPkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = true;
            android_sdk.accept_license = true;
          };
        };
        androidBuildToolsVersion = "35.0.0";
        androidNdkVersion = "29.0.14206865";
        androidComposition = androidPkgs.androidenv.composeAndroidPackages {
          # Keep only the API 33 emulator image; Tauri still compiles with SDK 36.
          repo =
            let
              repository = builtins.fromJSON (
                builtins.readFile "${nixpkgs}/pkgs/development/mobile/androidenv/repo.json"
              );
            in
            repository
            // {
              images = {
                "33" = repository.images."33";
              };
            };
          # Include the emulator platform alongside the SDK used for compilation.
          platformVersions = [
            "33"
            "36"
          ];
          buildToolsVersions = [ androidBuildToolsVersion ];
          includeNDK = true;
          ndkVersions = [ androidNdkVersion ];
          includeEmulator = true;
          includeSystemImages = true;
          systemImageTypes = [ "google_apis" ];
          abiVersions = [ "x86_64" ];
        };
        androidSdk = "${androidComposition.androidsdk}/libexec/android-sdk";

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

        devShells = {
          default = pkgs.mkShell {
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

          windows = pkgs.mkShell {
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
        // pkgs.lib.optionalAttrs (system == "x86_64-linux") {
          # Google's Linux NDK/emulator host binaries require x86_64 Linux.
          android = pkgs.mkShell {
            nativeBuildInputs = [
              pkgs.nodejs_24
              pkgs.yarn
              pkgs.jdk17
              pkgs.pkg-config
              androidRustToolchain
              androidComposition.androidsdk
            ];

            JAVA_HOME = pkgs.jdk17.home;
            ANDROID_HOME = androidSdk;
            ANDROID_SDK_ROOT = androidSdk;
            NDK_HOME = "${androidSdk}/ndk/${androidNdkVersion}";
            # Read by the Android Gradle project, including its library subprojects.
            TAURI_ANDROID_AAPT2 = "${androidSdk}/build-tools/${androidBuildToolsVersion}/aapt2";

            shellHook = ''
              unset CARGO_BUILD_TARGET
              export RUST_SRC_PATH="${androidRustToolchain}/lib/rustlib/src/rust/library"
            '';
          };
        };
      }
    );
}
