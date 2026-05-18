{
  description = "Nix flake for Surge - blazing fast TUI download manager";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      version = "0.8.5";
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEachSystem = f: nixpkgs.lib.genAttrs systems (system:
        f nixpkgs.legacyPackages.${system}
      );
    in
    {
      packages = forEachSystem (pkgs: rec {
        surge = pkgs.callPackage ./package.nix { src = self; inherit version; };
        default = surge;
      });

      overlays.default = final: _prev: {
        surge = final.callPackage ./package.nix { src = self; inherit version; };
      };

      nixosModules.default = { lib, pkgs, config, ... }: {
        options.programs.surge.enable = lib.mkEnableOption "surge download manager";

        config = lib.mkIf config.programs.surge.enable {
          nixpkgs.overlays = [ self.overlays.default ];
          environment.systemPackages = [ pkgs.surge ];
        };
      };
    };
}
