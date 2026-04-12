{
  description = "flaxx - Generic Flux app scaffolding tool";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "flaxx";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-YXjpRl3KVhLgi3QW275D1a/f9khfisMwC1RmZ5P3Pmc=";
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
          ];
        };
      }) // {
      overlays.default = final: prev: {
        flaxx = self.packages.${prev.system}.default;
      };
    };
}
