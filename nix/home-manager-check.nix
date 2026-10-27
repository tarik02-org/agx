# Evaluates the Home Manager module against stand-ins for the Home Manager
# options it uses, so the check needs no Home Manager input, and builds the
# rendered drop-in (validated by agx) and the activation script.
{
  pkgs,
  module,
}:

let
  lib = pkgs.lib.extend (
    _: _: {
      hm.dag.entryAfter = after: data: {
        inherit after data;
        before = [ ];
      };
    }
  );

  stubs =
    { config, ... }:
    {
      options = {
        lib = lib.mkOption {
          type = lib.types.attrsOf lib.types.attrs;
          default = { };
        };
        home = {
          homeDirectory = lib.mkOption { type = lib.types.str; };
          profileDirectory = lib.mkOption { type = lib.types.str; };
          packages = lib.mkOption { type = lib.types.listOf lib.types.package; };
          sessionPath = lib.mkOption { type = lib.types.listOf lib.types.str; };
          file = lib.mkOption {
            type = lib.types.attrsOf (
              lib.types.submodule { options.source = lib.mkOption { type = lib.types.path; }; }
            );
          };
          activation = lib.mkOption { type = lib.types.attrsOf lib.types.attrs; };
        };
        xdg = {
          enable = lib.mkOption {
            type = lib.types.bool;
            default = false;
          };
          configHome = lib.mkOption {
            type = lib.types.str;
            default = "${config.home.homeDirectory}/.config";
          };
          dataHome = lib.mkOption {
            type = lib.types.str;
            default = "${config.home.homeDirectory}/.local/share";
          };
        };
      };
    };

  eval = lib.evalModules {
    specialArgs = { inherit pkgs; };
    modules = [
      stubs
      module
      (
        { config, ... }:
        {
          home.homeDirectory = "/home/test";
          home.profileDirectory = "/home/test/.nix-profile";
          programs.agx = {
            enable = true;
            settings = {
              secrets.docs-key = {
                kind = "command";
                command = [
                  "echo"
                  "key"
                ];
              };
              mcp.docs = {
                url = "https://docs.example.com/mcp";
                headers.X-Api-Key = config.lib.agx.secret "docs-key";
              };
              harnesses.codex.environment.HOME_DIR = config.lib.agx.env "HOME";
              daemon.lease_seconds = 90;
            };
            harnesses.codex.package = pkgs.writeShellScriptBin "codex" "exit 0";
            harnesses.claude = { };
          };
        }
      )
    ];
  };

  cfg = eval.config;
  configDir =
    if pkgs.stdenv.hostPlatform.isDarwin then "Library/Application Support/agx" else ".config/agx";
  dropIn = cfg.home.file."${configDir}/agx.d/10-home-manager.yaml".source;
  root = cfg.home.file."${configDir}/agx.yaml".source;
  activation = pkgs.writeText "activation" (
    lib.concatMapStrings (entry: entry.data) (lib.attrValues cfg.home.activation)
  );
  links = lib.findFirst (p: p.name == "agx-harness-links") null cfg.home.packages;
in
pkgs.runCommand "agx-home-manager-module" { } ''
  grep -q 'X-Api-Key: !secret docs-key' ${dropIn}
  grep -q 'HOME_DIR: !env HOME' ${dropIn}
  grep -q 'executable: /nix/store/.*/bin/codex' ${dropIn}
  ! grep -q daemon ${dropIn}
  grep -q 'lease_seconds: 90' ${root}
  grep -q 'agx.d/\*.yaml' ${root}
  grep -q "XDG_DATA_HOME='\?/home/test/.local/share'\? .*" ${activation}
  grep -q 'does not list agx.d' ${activation}
  [ -L ${links}/bin/codex ] && [ -L ${links}/bin/claude ]
  [ ${lib.escapeShellArg (toString cfg.home.sessionPath)} = /home/test/.local/share/agx/shims ]
  touch $out
''
