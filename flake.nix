{
  description = "Nix flake for Surge - blazing fast TUI download manager";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEachSystem = f: nixpkgs.lib.genAttrs systems (system:
        f nixpkgs.legacyPackages.${system}
      );
    in
    {
      packages = forEachSystem (pkgs: {
        surge = pkgs.callPackage ./package.nix { };
        default = self.packages.${pkgs.system}.surge;
      });

      overlays.default = final: _prev: {
        surge = final.callPackage ./package.nix { };
      };

      # Applies the overlay so pkgs.surge is available system-wide.
      nixosModules.default = { pkgs, ... }: {
        nixpkgs.overlays = [ self.overlays.default ];
        environment.systemPackages = [ pkgs.surge ];
      };
    };
}
