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
    hash = "sha256-V4m5pBaN0Z0p2gjtuBU3YtT50fU0+ZjXV2r0dhT0ELo=";
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
