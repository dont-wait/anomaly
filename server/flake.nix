{
  description = "Anomaly";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let

      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forEachSystem = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      devShells = forEachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          runtimeLibs = with pkgs; [
            stdenv.cc.cc.lib
            zlib
          ];
        in
        {
          default = pkgs.mkShell {

            packages =
              with pkgs;
              [
                go
                gopls
                gotools
                gofumpt
                golangci-lint
                nodejs
                python3
                python3Packages.python-lsp-server
                python3Packages.pyls-isort
              ]
              ++ runtimeLibs;

            shellHook = ''
              export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath runtimeLibs}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}" 
              echo "Anomay development environment actived"
              go version
            '';
          };
        }
      );
    };
}
