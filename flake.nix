{
  description = "Directory-scoped skills, MCP servers, and instructions for coding agents";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        web = pkgs.callPackage ./nix/web.nix { };
        agx = pkgs.callPackage ./package.nix { inherit web; };
        default = agx;
      });

      apps = forAllSystems (pkgs: {
        default = {
          type = "app";
          program = pkgs.lib.getExe self.packages.${pkgs.stdenv.hostPlatform.system}.default;
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          inputsFrom = [ self.packages.${pkgs.stdenv.hostPlatform.system}.default ];
          packages = [
            pkgs.buf
            pkgs.golangci-lint
            pkgs.gopls
            pkgs.nodejs_24
            pkgs.pnpm_12
          ];
        };
      });

      homeManagerModules = {
        agx = import ./nix/home-manager.nix self;
        default = self.homeManagerModules.agx;
      };

      checks = forAllSystems (pkgs: {
        inherit (self.packages.${pkgs.stdenv.hostPlatform.system}) agx;
        home-manager-module = import ./nix/home-manager-check.nix {
          inherit pkgs;
          module = self.homeManagerModules.agx;
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
