{
  buildGoModule,
  lib,
}:

buildGoModule (finalAttrs: {
  pname = "agx";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./cmd
      ./gen/agx
      ./internal
    ];
  };

  vendorHash = "sha256-GhomrGes+mAKepA5ZJPl3o1CDQljkPFqvsOPGzYHpKA=";

  subPackages = [ "cmd/agx" ];

  ldflags = [
    "-s"
    "-w"
    "-X github.com/tarik02-org/agx/internal/daemon.Version=${finalAttrs.version}"
  ];

  doInstallCheck = true;
  installCheckPhase = ''
    runHook preInstallCheck
    "$out/bin/agx" --version >/dev/null
    runHook postInstallCheck
  '';

  meta = {
    description = "Directory-scoped skills, MCP servers, and instructions for coding agents";
    license = lib.licenses.mit;
    mainProgram = "agx";
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
})
