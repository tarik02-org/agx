{
  buildGoModule,
  lib,
}:

buildGoModule (finalAttrs: {
  pname = "weft";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./cmd
      ./gen/weft
      ./internal
    ];
  };

  vendorHash = "sha256-tT0ytn5Mvb0PCDFFHFZx30Pzbm2YCDSmDHTYeAkWv3w=";

  subPackages = [ "cmd/weft" ];

  ldflags = [
    "-s"
    "-w"
    "-X github.com/tarik02/weft/internal/daemon.Version=${finalAttrs.version}"
  ];

  doInstallCheck = true;
  installCheckPhase = ''
    runHook preInstallCheck
    "$out/bin/weft" --version >/dev/null
    runHook postInstallCheck
  '';

  meta = {
    description = "Directory-scoped skills, MCP connections, and instructions for coding agents";
    license = lib.licenses.mit;
    mainProgram = "weft";
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
})
