import { createFileRoute, redirect } from "@tanstack/react-router";
import * as Schema from "effect/Schema";

const SkillsSearch = Schema.Struct({
  skill: Schema.optional(Schema.String),
  file: Schema.optional(Schema.String),
});

// A skill is an item of the configuration; its page is the item page. Links from before keep
// working.
export const Route = createFileRoute("/skills/")({
  beforeLoad: ({ location }) => {
    const search = Schema.decodeUnknownSync(SkillsSearch)(location.search);
    throw search.skill === undefined
      ? redirect({ to: "/" })
      : redirect({
          to: "/config",
          search: {
            kind: "skills",
            id: search.skill,
            ...(search.file === undefined || search.file === "SKILL.md"
              ? {}
              : { path: search.file }),
          },
        });
  },
});
