# Evaluate the integration without building an entire NixOS system.
let
  flake = builtins.getFlake (toString ../.);
  system = "x86_64-linux";
  nixos = flake.inputs.nixpkgs.lib.nixosSystem {
    inherit system;
    modules = [
      flake.nixosModules.default
      {
        system.stateVersion = "26.05";
        programs.surge.enable = true;
        programs.surge.systemd.enable = true;
      }
    ];
  };
  package = nixos.pkgs.surge;
  service = nixos.config.systemd.services.surge.serviceConfig;
in
assert builtins.elem package nixos.config.environment.systemPackages;
assert service.ExecStart == "${package}/bin/surge server start --is-system-service";
assert package.version == flake.packages.${system}.surge.version;
{
  inherit (package) version;
  inherit (service) ExecStart;
}
