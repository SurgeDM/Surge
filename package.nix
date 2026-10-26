{
  lib,
  stdenvNoCC,
  fetchurl,
}: let
  release = builtins.fromJSON (builtins.readFile ./nix/release.json);
  inherit (release) version;
  archives = {
    x86_64-linux = "linux_amd64";
    aarch64-linux = "linux_arm64";
    x86_64-darwin = "darwin_amd64";
    aarch64-darwin = "darwin_arm64";
  };
  system = stdenvNoCC.hostPlatform.system;
in
  stdenvNoCC.mkDerivation {
    pname = "surge";
    inherit version;

    src = fetchurl {
      url = "https://github.com/SurgeDM/Surge/releases/download/v${version}/Surge_${version}_${archives.${system}}.tar.gz";
      hash = release.hashes.${system};
    };

    sourceRoot = ".";
    dontConfigure = true;
    dontBuild = true;
    dontStrip = true;
    dontPatchELF = true;

    installPhase = ''
      runHook preInstall
      install -Dm755 surge "$out/bin/surge"
      ln -s surge "$out/bin/Surge"
      install -Dm644 LICENSE "$out/share/licenses/surge/LICENSE"
      runHook postInstall
    '';

    doInstallCheck = true;
    installCheckPhase = ''
      runHook preInstallCheck
      test "$("$out/bin/surge" --version)" = "Surge v${version}"
      test "$("$out/bin/Surge" --version)" = "Surge v${version}"
      runHook postInstallCheck
    '';

    meta = {
      description = "Blazing fast TUI download manager built in Go";
      homepage = "https://github.com/SurgeDM/Surge";
      license = lib.licenses.mit;
      mainProgram = "surge";
      sourceProvenance = [lib.sourceTypes.binaryNativeCode];
      platforms = builtins.attrNames archives;
    };
  }
