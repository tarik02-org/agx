import * as NodeServices from "@effect/platform-node/NodeServices";
import * as Layer from "effect/Layer";

import { AgxClientLive } from "./AgxClient.ts";
import { Configuration } from "./Configuration.ts";
import { SkillDeployerLive } from "./SkillDeployer.ts";

export const runtime = (configuration: Configuration["Service"]) =>
  Layer.mergeAll(AgxClientLive, SkillDeployerLive).pipe(
    Layer.provideMerge(
      Layer.merge(NodeServices.layer, Layer.succeed(Configuration, configuration)),
    ),
  );
