{
  fetchPnpmDeps,
  lib,
  nodejs_24,
  pnpm,
  pnpmConfigHook,
  stdenvNoCC,
}:

# The web UI's static files, which the agx binary embeds.
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "agx-web";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.difference (lib.fileset.unions [
      ../LICENSE
      ../package.json
      ../pnpm-lock.yaml
      ../pnpm-workspace.yaml
      ../tsconfig.base.json
      ../tsconfig.json
      ../vite.config.ts
      ../packages/client
      ../packages/web
    ]) (lib.fileset.unions [
      (lib.fileset.maybeMissing ../packages/client/node_modules)
      (lib.fileset.maybeMissing ../packages/client/dist)
      (lib.fileset.maybeMissing ../packages/web/node_modules)
    ]);
  };

  # The UI and the client it builds on, without the other packages' dependencies.
  pnpmWorkspaces = [ "agx-web..." ];

  pnpmDeps = fetchPnpmDeps {
    inherit (finalAttrs)
      pname
      version
      src
      pnpmWorkspaces
      ;
    inherit pnpm;
    fetcherVersion = 4;
    # pnpm leaves an empty links directory in the store on macOS only; without it the store is
    # the same on every platform.
    preFixup = ''
      rmdir "$storePath/v11/links" 2>/dev/null || true
    '';
    hash = "sha256-vhd2BS4tCQnDmtc5Rh9CeQ+4M/pfl/v4pRwpTB78Nb4=";
  };

  nativeBuildInputs = [
    nodejs_24
    pnpm
    pnpmConfigHook
  ];

  buildPhase = ''
    runHook preBuild
    pnpm exec vp run agx-web#build
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    cp -r internal/ui/dist $out
    rm -f $out/.keep
    runHook postInstall
  '';
})
