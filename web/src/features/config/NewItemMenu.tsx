import { useNavigate } from "@tanstack/react-router";
import { GitBranchPlusIcon, PlusIcon } from "lucide-react";

import { Button } from "~/components/ui/button.tsx";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu.tsx";
import { kinds } from "~/features/config/kinds.ts";

/** Starts something new: a definition of any kind, or skills from a Git repository. */
export function NewItemMenu() {
  const navigate = useNavigate();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" size="icon" aria-label="New…" title="New…">
            <PlusIcon />
          </Button>
        }
      />
      <DropdownMenuContent align="end" className="w-auto">
        <DropdownMenuGroup>
          {kinds.map(({ kind, one }) => (
            <DropdownMenuItem
              key={kind}
              onClick={() => void navigate({ to: "/config", search: { kind, create: true } })}
            >
              {one}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem onClick={() => void navigate({ to: "/skills/add" })}>
            <GitBranchPlusIcon />
            Skills from Git…
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
