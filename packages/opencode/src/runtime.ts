import * as NodeServices from "@effect/platform-node/NodeServices";
import { AgxClientLive } from "agx-client/AgxClient";
import { AgxConnectionLive } from "agx-client/AgxConnection";
import * as Layer from "effect/Layer";

import { Configuration } from "./Configuration.ts";
import { SkillDeployerLive } from "./SkillDeployer.ts";

export const runtime = (configuration: Configuration["Service"]) =>
  Layer.mergeAll(
    AgxClientLive,
    AgxConnectionLive(configuration.connection),
    SkillDeployerLive,
  ).pipe(
    Layer.provideMerge(
      Layer.merge(NodeServices.layer, Layer.succeed(Configuration, configuration)),
    ),
  );
