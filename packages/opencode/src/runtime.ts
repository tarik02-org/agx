import * as NodeServices from "@effect/platform-node/NodeServices";
import { AgxClientLive } from "agx-client/AgxClient";
import { AgxConnectionLive } from "agx-client/AgxConnection";
import { SessionProviderClientLive } from "agx-client/SessionProviderClient";
import * as Layer from "effect/Layer";

import { Configuration } from "./Configuration.ts";
import { NativeSessionRoutingLive } from "./NativeSessionRouting.ts";
import { SkillDeployerLive } from "./SkillDeployer.ts";

export const runtime = (configuration: Configuration["Service"]) =>
  Layer.mergeAll(
    AgxClientLive,
    SessionProviderClientLive,
    AgxConnectionLive(configuration.connection),
    SkillDeployerLive,
    NativeSessionRoutingLive,
  ).pipe(
    Layer.provideMerge(
      Layer.merge(NodeServices.layer, Layer.succeed(Configuration, configuration)),
    ),
  );
