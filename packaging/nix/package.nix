{
  perSystem = { pkgs, config, ... }: {
    packages.surge = (pkgs.callPackage ../../package.nix { }).overrideAttrs (finalAttrs: {
      passthru.services.default = {
        imports = [ (pkgs.lib.modules.importApply ./service.nix { }) ];
        surge.package = pkgs.lib.mkDefault finalAttrs.finalPackage;
      };
    });
    packages.default = config.packages.surge;
    checks.package = config.packages.surge;
  };
}
