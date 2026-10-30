{
  buildGoModule,
  installShellFiles,
  lib,
  stdenv,
  # The flake passes its commit.
  rev ? null,
}:

buildGoModule (finalAttrs: {
  pname = "agx";
  # Until the first tag.
  version = "0.0.0-dev" + lib.optionalString (rev != null) "+${rev}";

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

  vendorHash = "sha256-5eg3SJ5vKDiHuQGvb3llGD0BjJuE5te2c8M86YPrwUY=";

  subPackages = [ "cmd/agx" ];

  nativeBuildInputs = [ installShellFiles ];

  # Completions come from the built binary, so they need one that runs here.
  postInstall = lib.optionalString (stdenv.buildPlatform.canExecute stdenv.hostPlatform) ''
    installShellCompletion --cmd agx \
      --bash <("$out/bin/agx" completion bash) \
      --fish <("$out/bin/agx" completion fish) \
      --zsh <("$out/bin/agx" completion zsh)
  '';

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
