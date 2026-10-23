{ moduleWithSystem, ... }:
{
  flake = {
    homeModules.default = moduleWithSystem (
      { config, ... }:
      let
        flakeConfig = config;
      in
      {
        config,
        lib,
        pkgs,
        ...
      }:
      let
        cfg = config.programs.surge;
        toml = pkgs.formats.toml { };
        json = pkgs.formats.json { };
      in
      {
        options.programs.surge = {
          enable = lib.mkEnableOption "surge";

          package = lib.mkOption {
            description = "Package to use for surge";
            default = flakeConfig.packages.default;
            type = lib.types.package;
          };

          settings = lib.mkOption {
            type = lib.types.nullOr toml.type;
            description = "Configuration for Surge, as defined in https://github.com/SurgeDM/Surge/blob/main/docs/SETTINGS.md";
            default = null;
            example = {
              general = {
                default_download_dir = "/path/to/downloads";
                theme = 2;
              };
              network = {
                max_connections_per_host = 16;
              };
              performance = {
                max_task_retries = 5;
              };
              categories = {
                category_enabled = true;
              };
            };
          };

          keymap = lib.mkOption {
            type = lib.types.nullOr json.type;
            description = "Keymap configuration for Surge";
            default = null;
            example = {
              dashboard = {
                Quit = {
                  keys = [
                    "ctrl+c"
                    "ctrl+q"
                  ];
                  help = "quit";
                };
                Up = {
                  keys = [
                    "up"
                    "k"
                  ];
                  help = "up";
                };
              };
            };
          };

          headless = {
            enable = lib.mkEnableOption "surge headless server";

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
        };

        config = lib.mkIf cfg.enable (
          lib.mkMerge [
            {
              home.packages = [ cfg.package ];

              xdg.configFile = {
                "surge/settings.toml" = lib.mkIf (!isNull cfg.settings) {
                  source = toml.generate "settings.toml" cfg.settings;
                };
                "surge/keymap.json" = lib.mkIf (!isNull cfg.keymap) {
                  source = json.generate "keymap.json" cfg.keymap;
                  force = true;
                };
              };
            }
            (lib.mkIf cfg.headless.enable {
              home.services.surge = {
                imports = [ cfg.package.services.default ];
                surge = {
                  inherit (cfg) package;
                  inherit (cfg.headless) extraArgs token;
                };
              };
            })
          ]
        );

        meta.maintainers = with lib.maintainers; [
          ErmitaVulpe
        ];
      }
    );
  };
}
