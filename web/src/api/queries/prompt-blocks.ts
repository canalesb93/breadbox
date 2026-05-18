import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

// PromptBlockGroup mirrors the Go-side taxonomy in
// internal/service/prompt_blocks.go::promptBlockGroupFor. Bumping this
// requires a coordinated change on both sides.
export type PromptBlockGroup =
  | "strategy"
  | "depth"
  | "integration"
  | "knowledge";

export interface PromptBlock {
  id: string;
  title: string;
  description: string;
  /** Kebab-case Lucide icon name (e.g. "calendar-check"), or empty
   *  when the block file omits the frontmatter `icon` field. */
  icon?: string;
  group: PromptBlockGroup;
  content: string;
}

// usePromptBlocks loads the embedded library once per session. The
// blocks ship inside the Go binary at build time so the response is
// effectively static; staleTime is generous to suppress refetches as
// the user navigates between the builder and other routes.
export function usePromptBlocks() {
  return useQuery({
    queryKey: ["agents", "prompt-blocks"],
    queryFn: () => api<PromptBlock[]>("/api/v1/agents/prompt-blocks"),
    staleTime: 60 * 60 * 1000,
  });
}
