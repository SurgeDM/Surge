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
        nativeDarwin = pkgs.stdenv.hostPlatform.isDarwin && !config.xdg.enable;
        configDir =
          if nativeDarwin then
            "${config.home.homeDirectory}/Library/Application Support/surge"
          else
            "${config.xdg.configHome}/surge";
        stateDir =
          if nativeDarwin then
            "${config.home.homeDirectory}/Library/Application Support/surge"
          else
            "${config.xdg.stateHome}/surge";
        rawServerArgs = [
          (lib.getExe cfg.package)
          "server"
          "start"
          "--managed-service"
        ]
        ++ cfg.headless.extraArgs;
        checkedServer = pkgs.writeShellScript "surge-server" ''
          export PATH=${lib.makeBinPath [ pkgs.gnugrep ]}:$PATH
          ${builtins.readFile ./check-token.sh}
        '';
        serverArgs =
          if isNull cfg.headless.tokenFile then
            rawServerArgs
          else
            [
              (toString checkedServer)
              cfg.headless.tokenFile
              "${stateDir}/token"
            ]
            ++ rawServerArgs;
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
            description = "Declarative settings for Surge (https://github.com/SurgeDM/Surge/blob/main/docs/SETTINGS.md). When set, these replace runtime edits on each Home Manager activation.";
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
            description = "Declarative keymap for Surge. When set, this replaces runtime edits on each Home Manager activation.";
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

            tokenFile = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              description = "Absolute path to a protected runtime file containing the API token (for example from sops-nix). Startup fails if the override is unavailable, unreadable, or empty. Its contents are never copied into the Nix store. If null, Surge uses its persisted or auto-generated token; run `surge token` to view it.";
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

              home.file = {
                "${configDir}/settings.toml" = lib.mkIf (!isNull cfg.settings) {
                  source = toml.generate "settings.toml" cfg.settings;
                  # Surge saves settings by replacing this symlink.
                  force = true;
                };
                "${configDir}/keymap.json" = lib.mkIf (!isNull cfg.keymap) {
                  source = json.generate "keymap.json" cfg.keymap;
                  force = true;
                };
              };
            }
            (lib.mkIf cfg.headless.enable {
              assertions = [
                {
                  assertion = cfg.headless.tokenFile == null || lib.hasPrefix "/" cfg.headless.tokenFile;
                  message = "programs.surge.headless.tokenFile must be an absolute runtime path.";
                }
              ];
              home.file."${stateDir}/token" = lib.mkIf (!isNull cfg.headless.tokenFile) {
                force = true;
                source = config.lib.file.mkOutOfStoreSymlink cfg.headless.tokenFile;
              };
              systemd.user.services.surge = lib.mkIf pkgs.stdenv.hostPlatform.isLinux {
                Unit = {
                  Description = "Surge downloader headless server";
                  After = [ "network-online.target" ];
                  Wants = [ "network-online.target" ];
                  X-Restart-Triggers = lib.mkIf (!isNull cfg.settings) [
                    config.home.file."${configDir}/settings.toml".source
                  ];
                };
                Service = {
                  ExecStart = lib.replaceStrings [ "%" "$" ] [ "%%" "$$" ] (lib.escapeShellArgs serverArgs);
                  Environment = [
                    "XDG_CONFIG_HOME=${config.xdg.configHome}"
                    "XDG_STATE_HOME=${config.xdg.stateHome}"
                  ];
                  WorkingDirectory = config.home.homeDirectory;
                  Restart = "on-failure";
                  NoNewPrivileges = true;
                  PrivateTmp = true;
                };
                Install.WantedBy = [ "default.target" ];
              };
              launchd.agents.surge = lib.mkIf pkgs.stdenv.hostPlatform.isDarwin {
                enable = true;
                config = {
                  ProgramArguments = serverArgs;
                  RunAtLoad = true;
                  KeepAlive = {
                    SuccessfulExit = false;
                  };
                  WorkingDirectory = config.home.homeDirectory;
                  EnvironmentVariables =
                    (lib.optionalAttrs config.xdg.enable {
                      XDG_CONFIG_HOME = config.xdg.configHome;
                      XDG_STATE_HOME = config.xdg.stateHome;
                    })
                    // (lib.optionalAttrs (!isNull cfg.settings) {
                      # Changing the plist makes Home Manager reload the agent.
                      SURGE_SETTINGS_SOURCE = toString config.home.file."${configDir}/settings.toml".source;
                    });
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
