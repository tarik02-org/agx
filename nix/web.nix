{
  fetchPnpmDeps,
  lib,
  nodejs_24,
  pnpm_12,
  pnpmConfigHook,
  stdenvNoCC,
}:

# The web UI's static files, which the agx binary embeds.
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "agx-web";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../web;
    fileset = lib.fileset.difference ../web (lib.fileset.maybeMissing ../web/node_modules);
  };

  pnpmDeps = fetchPnpmDeps {
    inherit (finalAttrs) pname version src;
    pnpm = pnpm_12;
    fetcherVersion = 4;
    # pnpm 12 leaves an empty links directory in the store on macOS only; without it the store
    # is the same on every platform.
    preFixup = ''
      rmdir "$storePath/v11/links" 2>/dev/null || true
    '';
    hash = "sha256-YUsAj63t2krnNDYICV/5RHl6osDmr+zD+XfCn1hfgSI=";
  };

  nativeBuildInputs = [
    nodejs_24
    pnpm_12
    pnpmConfigHook
  ];

  buildPhase = ''
    runHook preBuild
    pnpm exec vp build --outDir dist --emptyOutDir
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    cp -r dist $out
    runHook postInstall
  '';
})
