import { Skill } from "@opencode/schema/skill";
import * as Context from "effect/Context";
import * as Crypto from "effect/Crypto";
import * as Effect from "effect/Effect";
import * as Hex from "effect/encoding/Hex";
import * as FileSystem from "effect/FileSystem";
import * as Layer from "effect/Layer";
import * as Path from "effect/Path";
import * as Schema from "effect/Schema";
import * as Scope from "effect/Scope";
import { parse, YAMLParseError } from "yaml";

import { IntegrationError } from "./error.ts";
import { Frontmatter, type SkillTree } from "./schema.ts";

export class SkillDeployer extends Context.Service<
  SkillDeployer,
  {
    readonly prepare: (tree: SkillTree) => Effect.Effect<Skill.Info, IntegrationError>;
  }
>()("agx-opencode/SkillDeployer") {}

export const makeSkillDeployer = Effect.fn("makeSkillDeployer")(function* () {
  const fs = yield* FileSystem.FileSystem;
  const path = yield* Path.Path;
  const crypto = yield* Crypto.Crypto;
  const scope = yield* Effect.scope;
  const deployed = new Map<string, string>();
  let root: string | null = null;

  const deploy = Effect.fn("SkillDeployer.deploy")(
    function* (tree: SkillTree) {
      const existing = deployed.get(tree.digest);
      if (existing !== undefined) {
        return path.join(existing, "SKILL.md");
      }
      if (root === null) {
        root = yield* fs
          .makeTempDirectoryScoped({ prefix: "agx-opencode-" })
          .pipe(Effect.provideService(Scope.Scope, scope));
      }
      const directory = path.join(root, tree.digest);
      yield* fs.makeDirectory(directory, { recursive: true, mode: 0o700 });
      for (const name of tree.directories) {
        yield* fs.makeDirectory(path.join(directory, ...name.split("/")), {
          recursive: true,
          mode: 0o700,
        });
      }
      for (const file of tree.files) {
        const destination = path.join(directory, ...file.path.split("/"));
        yield* fs.makeDirectory(path.dirname(destination), { recursive: true, mode: 0o700 });
        yield* fs.writeFile(destination, file.content, { mode: 0o600 });
        if (file.executable) {
          yield* fs.chmod(destination, 0o700);
        }
      }
      deployed.set(tree.digest, directory);
      return path.join(directory, "SKILL.md");
    },
    Effect.catchTag("PlatformError", (cause) =>
      Effect.fail(
        new IntegrationError({ operation: "skill deployment", reason: "filesystem", cause }),
      ),
    ),
  );

  const prepare = Effect.fn("SkillDeployer.prepare")(function* (tree: SkillTree) {
    const document = tree.files.find((file) => file.path === "SKILL.md");
    if (document === undefined) {
      return yield* new IntegrationError({ operation: "skill document", reason: "content" });
    }
    const text = new TextDecoder()
      .decode(document.content)
      .replace(/^\uFEFF/, "")
      .replace(/\r\n/g, "\n");
    let content = text;
    let frontmatter: unknown = {};
    if (text.startsWith("---\n")) {
      const end = text.indexOf("\n---", 4);
      if (end === -1) {
        return yield* new IntegrationError({ operation: "skill frontmatter", reason: "content" });
      }
      frontmatter = yield* Effect.try((): unknown => parse(text.slice(4, end)) ?? {}).pipe(
        Effect.catchTag("UnknownError", (error) => {
          const cause = error.cause;
          const code = Schema.is(Schema.instanceOf(YAMLParseError))(cause)
            ? cause.code
            : // yaml exposes its alias-expansion limit without a dedicated error type or code.
              Schema.is(Schema.instanceOf(ReferenceError))(cause) &&
                cause.message === "Excessive alias count indicates a resource exhaustion attack"
              ? "EXCESSIVE_ALIASES"
              : null;
          return code === null
            ? Effect.die(cause)
            : Effect.fail(
                new IntegrationError({
                  operation: "skill frontmatter",
                  reason: "content",
                  cause: { kind: "yaml", code },
                }),
              );
        }),
      );
      content = text.slice(end + 4).replace(/^\n/, "");
    }
    const metadata = yield* Schema.decodeUnknownEffect(Frontmatter)(frontmatter).pipe(
      Effect.catchTag("SchemaError", (cause) =>
        Effect.fail(
          new IntegrationError({ operation: "skill metadata", reason: "content", cause }),
        ),
      ),
    );
    const hash = yield* crypto
      .digest("SHA-256", new TextEncoder().encode(tree.id))
      .pipe(
        Effect.catchTag("PlatformError", (cause) =>
          Effect.fail(
            new IntegrationError({ operation: "skill identity", reason: "content", cause }),
          ),
        ),
      );
    const id = Skill.ID.make(`agx-${tree.name}-${Hex.encode(hash).slice(0, 12)}`);
    const hasAssets =
      tree.files.some((file) => file.path !== "SKILL.md") || tree.directories.length > 0;
    const location = hasAssets ? yield* deploy(tree) : `skill://${id}/SKILL.md`;
    return Skill.Info.make({
      id,
      name: Skill.Name.make(tree.name),
      ...(metadata.description === undefined ? {} : { description: metadata.description }),
      autoinvoke: metadata["disable-model-invocation"] !== true,
      path: Skill.Info.fields.path.make(location),
      content,
    });
  });
  return SkillDeployer.of({ prepare });
});

export const SkillDeployerLive = Layer.effect(SkillDeployer, makeSkillDeployer());
