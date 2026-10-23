# Non-module dependencies (`importApply`)
{ }:

# Service module
{
  config,
  lib,
  options,
  ...
}:
let
  cfg = config.surge;
in
{
  _class = "service";

  meta.maintainers = with lib.maintainers; [ ErmitaVulpe ];

  options.surge = {
    package = lib.mkOption {
      description = "Package to use for surge";
      defaultText = "The package that provided this module.";
      type = lib.types.package;
    };

    token = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      description = "Api token for the server. If null, will use an auto-generated. To see what it is, run `surge token`";
      default = null;
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Extra arguments to pass to `surge server`";
    };
  };

  config = {
    process = {
      argv = [
        (lib.getExe cfg.package)
        "server"
        "start"
        "--managed-service"
      ]
      ++ lib.optionals (cfg.token != null) [
        "--token"
        cfg.token
      ]
      ++ cfg.extraArgs;
    };
  }
  // lib.optionalAttrs (options ? systemd) (
    let
      serviceCapabilities = [ "" ];
    in
    {
      systemd.mainExecStart = config.systemd.lib.escapeSystemdExecArgs config.process.argv;

      systemd.service = {
        description = "Surge downloader headless server";

        wantedBy = [ "multi-user.target" ];
        after = [ "network-online.target" ];
        wants = [ "network-online.target" ];

        serviceConfig = {
          Restart = "on-failure";
          ProtectSystem = "full";
          WorkingDirectory = "%h";

          # Hardening
          MemoryDenyWriteExecute = true;
          NoNewPrivileges = true;
          CapabilityBoundingSet = serviceCapabilities;
          AmbientCapabilities = serviceCapabilities;
          SystemCallFilter = "@system-service";
          ProtectProc = "noaccess";
          PrivateTmp = "yes";
        };
      };
    }
  )
  // lib.optionalAttrs (options ? launchd) {
    launchd.mainExecStart = config.process.argv;
  };
}
