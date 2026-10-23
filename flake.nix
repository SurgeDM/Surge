{
  description = "Nix flake for Surge - blazing fast TUI download manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-darwin"
        "x86_64-linux"
      ];
      imports = [
        ./packaging/nix/homeModule.nix
        ./packaging/nix/package.nix
      ];
      perSystem = { pkgs, ... }: {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
          ];
        };
      };
      flake.nixosModules.default = throw ''
        The Surge downloader nixosModule got removed in favor of the home-manager module.
      '';
    };
}
