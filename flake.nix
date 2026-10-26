{
  description = "Nix flake for Surge - blazing fast TUI download manager";

  # The 26.05 branch supports all four release platforms, including Intel macOS.
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = ["x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin"];
    forEachSystem = f:
      nixpkgs.lib.genAttrs systems (
        system:
          f nixpkgs.legacyPackages.${system}
      );
  in {
    packages = forEachSystem (pkgs: rec {
      surge = pkgs.callPackage ./package.nix {};
      default = surge;
    });

    checks = forEachSystem (pkgs: {
      package = self.packages.${pkgs.stdenv.hostPlatform.system}.surge;
    });

    overlays.default = final: _prev: {
      surge = final.callPackage ./package.nix {};
    };

    nixosModules.default = {
      lib,
      pkgs,
      config,
      ...
    }: let
      cfg = config.programs.surge;
    in {
      options.programs.surge = {
        enable = lib.mkEnableOption "surge download manager";
        systemd.enable = lib.mkEnableOption "systemd service";
      };

      config = lib.mkIf cfg.enable {
        nixpkgs.overlays = [self.overlays.default];
        environment.systemPackages = [pkgs.surge];
        systemd.services.surge = lib.mkIf cfg.systemd.enable {
          description = "Surge Download Manager";

          wantedBy = ["multi-user.target"];
          after = ["network-online.target"];
          wants = ["network-online.target"];

          serviceConfig = {
            ExecStart = "${pkgs.surge}/bin/surge server start --is-system-service";

            Restart = "on-failure";
            RestartSec = "5s";
          };
        };
      };
    };
  };
}
