{
  description = "flaxx - Generic Flux app scaffolding tool";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      # Read version from VERSION file, fall back to git short rev or "dev"
      version = let
        versionFile = builtins.readFile ./VERSION;
        trimmed = builtins.replaceStrings [ "\n" "\r" " " ] [ "" "" "" ] versionFile;
      in
        if trimmed != "" then trimmed
        else if (self ? shortRev) then self.shortRev
        else "dev";

      commit = if (self ? rev) then self.rev else "dirty";

      # Format lastModifiedDate: YYYYMMDDHHMMSS -> YYYY-MM-DDTHH:MM:SSZ
      date = let
        raw = self.lastModifiedDate or "19700101000000";
        year = builtins.substring 0 4 raw;
        month = builtins.substring 4 2 raw;
        day = builtins.substring 6 2 raw;
        hour = builtins.substring 8 2 raw;
        min = builtins.substring 10 2 raw;
        sec = builtins.substring 12 2 raw;
      in "${year}-${month}-${day}T${hour}:${min}:${sec}Z";

      # Package builder function - used by both overlay and packages output
      mkFlaxx = pkgs: pkgs.buildGoModule {
        pname = "flaxx";
        inherit version;

        src = ./.;

        vendorHash = "sha256-cuJpBV3ffQ43nRO439ED5A7ey5Z4q8hBHR/rPFfHPNw=";

        env.CGO_ENABLED = 0;

        nativeBuildInputs = [ pkgs.installShellFiles ];

        ldflags = [
          "-s"
          "-w"
          "-X github.com/xx4h/flaxx/cmd.version=${version}"
          "-X github.com/xx4h/flaxx/cmd.commit=${commit}"
          "-X github.com/xx4h/flaxx/cmd.date=${date}"
        ];

        postInstall = pkgs.lib.optionalString
          (pkgs.stdenv.buildPlatform.canExecute pkgs.stdenv.hostPlatform) ''
          installShellCompletion --cmd flaxx \
            --bash <($out/bin/flaxx completion bash) \
            --zsh  <($out/bin/flaxx completion zsh) \
            --fish <($out/bin/flaxx completion fish)
        '';

        meta = with pkgs.lib; {
          description = "Generic scaffolding and maintenance tool for FluxCD GitOps repositories";
          homepage = "https://github.com/xx4h/flaxx";
          license = licenses.asl20;
          maintainers = [ ];
          mainProgram = "flaxx";
        };
      };
    in
    {
      overlays.default = final: prev: {
        flaxx = mkFlaxx final;
      };

      homeManagerModules.default = import ./nix/hm-module.nix self;
      homeManagerModules.flaxx = self.homeManagerModules.default;
    }
    //
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          flaxx = mkFlaxx pkgs;
          default = self.packages.${system}.flaxx;
        };

        apps = {
          flaxx = flake-utils.lib.mkApp {
            drv = self.packages.${system}.flaxx;
          };
          default = self.apps.${system}.flaxx;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            golangci-lint
            goreleaser
            go-task
            gotools
            editorconfig-checker
            prettier
          ];

          shellHook = ''
            echo "flaxx development shell"
            echo "Go: $(go version)"
          '';
        };
      }
    );
}
