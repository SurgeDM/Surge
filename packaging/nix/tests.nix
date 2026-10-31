# Run: nix eval --impure --json --file packaging/nix/tests.nix
let
  surge = builtins.getFlake ("path:" + toString ../..);
  homeManager = builtins.getFlake "github:nix-community/home-manager/24f2190587513b9c2f73db112a17844ae912d965";
  lib = surge.inputs.nixpkgs.lib;
  check =
    system:
    let
      pkgs = surge.inputs.nixpkgs.legacyPackages.${system};
      darwin = pkgs.stdenv.hostPlatform.isDarwin;
      homeDir = if darwin then "/Users/alice" else "/home/alice";
      nativeDir =
        if darwin then "${homeDir}/Library/Application Support/surge" else "${homeDir}/.config/surge";
      package = pkgs.hello;
      evaluate =
        overrides:
        (homeManager.lib.homeManagerConfiguration {
          inherit pkgs;
          modules = [
            surge.homeModules.default
            {
              home.username = "alice";
              home.homeDirectory = homeDir;
              home.stateVersion = "26.05";
              programs.surge = {
                enable = true;
                inherit package;
                headless.enable = true;
              };
            }
            overrides
          ];
        }).config;
      defaults = evaluate { };
      configured = evaluate {
        programs.surge.settings.general.theme = 2;
        programs.surge.keymap.dashboard.Quit.keys = [ "q" ];
        programs.surge.headless.tokenFile = "/run/secrets/surge-token";
      };
      changed = evaluate {
        programs.surge.settings.general.theme = 3;
      };
      xdg = evaluate {
        xdg.enable = true;
        xdg.configHome = "${homeDir}/custom-config";
        xdg.stateHome = "${homeDir}/custom-state";
        programs.surge.settings.general.theme = 2;
        programs.surge.headless.tokenFile = "/run/secrets/surge-token";
      };
      tui = evaluate {
        programs.surge.headless.enable = lib.mkForce false;
        programs.surge.settings.general.theme = 2;
      };
      disabled = evaluate { programs.surge.enable = lib.mkForce false; };
      invalid =
        builtins.tryEval
          (evaluate { programs.surge.headless.tokenFile = "relative-token"; }).home.activationPackage;
      trigger =
        c:
        if darwin then
          c.launchd.agents.surge.config.EnvironmentVariables.SURGE_SETTINGS_SOURCE or null
        else
          c.systemd.user.services.surge.Unit.X-Restart-Triggers;
      argv =
        if darwin then
          configured.launchd.agents.surge.config.ProgramArguments
        else
          configured.systemd.user.services.surge.Service.ExecStart;
    in
    assert builtins.all (a: a.assertion) defaults.assertions;
    assert builtins.all (a: a.assertion) configured.assertions;
    assert !invalid.success;
    assert builtins.elem package defaults.home.packages;
    assert !(defaults.home.file ? "${nativeDir}/settings.toml");
    assert !(defaults.home.file ? "${nativeDir}/token");
    assert configured.home.file."${nativeDir}/settings.toml".force;
    assert configured.home.file."${nativeDir}/keymap.json".force;
    assert
      configured.home.file ? "${if darwin then nativeDir else "${homeDir}/.local/state/surge"}/token";
    assert xdg.home.file ? "${homeDir}/custom-config/surge/settings.toml";
    assert xdg.home.file ? "${homeDir}/custom-state/surge/token";
    assert tui.home.file ? "${nativeDir}/settings.toml";
    assert !(tui.systemd.user.services ? surge);
    assert !(tui.launchd.agents ? surge);
    assert !(disabled.systemd.user.services ? surge);
    assert !(disabled.launchd.agents ? surge);
    assert trigger defaults == (if darwin then null else [ ]);
    assert trigger configured != trigger changed;
    assert lib.hasInfix (builtins.unsafeDiscardStringContext "${package}/bin/hello") (
      builtins.toJSON argv
    );
    assert lib.hasInfix "surge-server" (builtins.toJSON argv);
    assert lib.hasInfix "/run/secrets/surge-token" (builtins.toJSON argv);
    assert !(lib.hasInfix "--token" (builtins.toJSON argv));
    assert
      if darwin then
        !(configured.systemd.user.services ? surge)
      else
        !(configured.launchd.agents ? surge);
    true;
in
lib.genAttrs [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ] check
