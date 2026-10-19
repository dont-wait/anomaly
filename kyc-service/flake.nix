{
  description = "Anomaly Python service development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        python = pkgs.python311;
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.docker-client
            pkgs.python311Packages.pip
            pkgs.python311Packages.virtualenv
            pkgs.ruff
            pkgs.ffmpeg-headless
            python
          ];

          LD_LIBRARY_PATH = pkgs.lib.makeLibraryPath [
            pkgs.stdenv.cc.cc.lib
            pkgs.zlib
            pkgs.glib
            pkgs.libglvnd
            pkgs.portaudio
            pkgs.libxcb
          ];

          shellHook = ''
            echo "Anomaly kyc-service development environment activated"
            python --version
            docker --version
            ruff --version
          '';
        };
      }
    );
}
