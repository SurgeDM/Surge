{
  description = "Nix flake for Surge - blazing fast TUI download manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
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
      flake.overlays.default = final: _prev: {
        surge = final.callPackage ./package.nix { };
      };
      flake.nixosModules.default =
        {
          config,
          lib,
          pkgs,
          ...
        }:
        {
          options.programs.surge = {
            enable = lib.mkEnableOption "Surge download manager";
            systemd.enable = lib.mkEnableOption "legacy system service";
          };
          config = lib.mkIf config.programs.surge.enable {
            environment.systemPackages = [ (pkgs.callPackage ./package.nix { }) ];
            assertions = [
              {
                assertion = !config.programs.surge.systemd.enable;
                message = "Surge runs as a user service. Use the Home Manager programs.surge.headless.enable option instead of programs.surge.systemd.enable.";
              }
            ];
          };
        };
    };
}
