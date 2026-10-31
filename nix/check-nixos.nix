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
      }
    ];
  };
  package = flake.packages.${system}.surge;
in
assert builtins.any (p: p.drvPath == package.drvPath) nixos.config.environment.systemPackages;
assert !(nixos.config.systemd.services ? surge);
assert package.version == flake.packages.${system}.surge.version;
{
  inherit (package) version;
}
