{ lib
, buildGoModule
, fetchFromGitHub
}:

buildGoModule rec {
  pname = "surge";
  version = "0.8.5";

  src = fetchFromGitHub {
    owner = "SurgeDM";
    repo = "Surge";
    rev = "v${version}";
    hash = "sha256-ZQeShqNf/vhD5IoZp2grNo0YBzAObIXZIw2kQIaPKWc=";
  };

  vendorHash = "sha256-XHsp2zxLOh9FB93w/g24M7II0yseOUXQGLFkX9BG96A=";

  subPackages = [ "." ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  # Tests write to $HOME/.config; redirect to a writable tmpdir.
  preCheck = ''
    export HOME=$TMPDIR
  '';

  meta = {
    description = "Blazing fast TUI download manager built in Go";
    homepage = "https://github.com/SurgeDM/Surge";
    license = lib.licenses.mit;
    mainProgram = "surge";
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
}
