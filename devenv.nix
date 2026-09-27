{
  pkgs,
  lib,
  config,
  inputs,
  ...
}:

{
  languages.go.enable = true;
  languages.go.version = "1.27.1";

  overlays = [
    (final: prev:
      let unstable = import inputs.nixpkgs-unstable { inherit (prev) system; };
      in {
        inherit (unstable) golangci-lint;

        # doCheck off: staticcheck's own suite is not a signal about this repo,
        # and honnef.co/go/tools/go/ir's tests get OOM-killed on the
        # linux-small Buildkite agent that builds the toolchain image
        # (`signal: killed` ~140s in, no assertion output). Cheaper and
        # deterministic to not run vendored-tool tests in the image build.
        # Verified `.overrideAttrs` survives devenv's later
        # `.override { buildGoModule = …; }` (buildWithSpecificGo) — doCheck
        # stays false. If ANOTHER derivation starts OOMing in that image, do
        # not add a second per-package opt-out: move the "Build devenv image"
        # step off queue linux-small, or throttle it with
        # `--nix-option cores 2 --nix-option max-jobs 1`.
        go-tools = unstable.go-tools.overrideAttrs (_: { doCheck = false; });
      })
  ];

  # https://devenv.sh/basics/
  env.GREET = "devenv";

  # https://devenv.sh/packages/
  packages = [ 
    pkgs.git
    pkgs.golangci-lint
    pkgs.coreutils
  ];

  # https://devenv.sh/languages/
  # languages.rust.enable = true;

  # https://devenv.sh/processes/
  # processes.dev.exec = "${lib.getExe pkgs.watchexec} -n -- ls -la";

  # https://devenv.sh/services/
  # services.postgres.enable = true;

  # https://devenv.sh/scripts/
  scripts.hello.exec = ''
    echo hello from $GREET
  '';

  # https://devenv.sh/basics/
  enterShell = ''
    hello         # Run scripts directly
    git --version # Use packages
  '';

  # https://devenv.sh/tasks/
  # tasks = {
  #   "myproj:setup".exec = "mytool build";
  #   "devenv:enterShell".after = [ "myproj:setup" ];
  # };

  # https://devenv.sh/tests/
  enterTest = ''
    echo "Running tests"
    git --version | grep --color=auto "${pkgs.git.version}"
  '';

  # https://devenv.sh/git-hooks/
  # git-hooks.hooks.shellcheck.enable = true;

  # See full reference at https://devenv.sh/reference/options/
}
