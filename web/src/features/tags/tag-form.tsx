import {
  useForm,
  useWatch,
  type Resolver,
  type SubmitHandler,
} from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNavigate } from "@tanstack/react-router";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { IconPicker } from "@/components/icon-picker";
import { ColorPicker } from "@/components/color-picker";
import { TagChip } from "@/components/tag-chip";
import { useCreateTag, useUpdateTag } from "@/api/queries/tags";
import { withMutationToast } from "@/lib/mutation-toast";
import type { Tag } from "@/api/types";

const SLUG_RE = /^[a-z0-9]([a-z0-9\-:]*[a-z0-9])?$/;

const createSchema = z.object({
  slug: z
    .string()
    .min(1, "Slug is required")
    .max(64, "Keep slug under 64 characters")
    .regex(SLUG_RE, "Lowercase letters, digits, hyphens, and colons only"),
  display_name: z
    .string()
    .min(1, "Display name is required")
    .max(128, "Keep it under 128 characters"),
  description: z.string().max(256, "Keep under 256 characters"),
  icon: z.string().nullable(),
  color: z.string().nullable(),
});

export type TagFormValues = z.infer<typeof createSchema>;

interface TagFormProps {
  mode: "create" | "edit";
  tag?: Tag;
}

export function TagForm({ mode, tag }: TagFormProps) {
  const navigate = useNavigate();
  const create = useCreateTag();
  const update = useUpdateTag();

  const form = useForm<TagFormValues>({
    resolver: zodResolver(createSchema) as Resolver<TagFormValues>,
    defaultValues: {
      slug: tag?.slug ?? "",
      display_name: tag?.display_name ?? "",
      description: tag?.description ?? "",
      icon: tag?.icon ?? null,
      color: tag?.color ?? null,
    },
  });

  const [slug, displayName, icon, color] = useWatch({
    control: form.control,
    name: ["slug", "display_name", "icon", "color"],
  });

  const onSubmit: SubmitHandler<TagFormValues> = async (values) => {
    if (mode === "create") {
      const ok = await withMutationToast(
        () =>
          create.mutateAsync({
            slug: values.slug,
            display_name: values.display_name,
            description: values.description || undefined,
            icon: values.icon,
            color: values.color,
          }),
        { success: "Tag created." },
      );
      if (ok) navigate({ to: "/tags" });
    } else if (tag) {
      const ok = await withMutationToast(
        () =>
          update.mutateAsync({
            slug: tag.slug,
            input: {
              display_name: values.display_name,
              description: values.description,
              icon: values.icon,
              color: values.color,
            },
          }),
        { success: "Tag updated." },
      );
      if (ok) navigate({ to: "/tags" });
    }
  };

  const isPending = create.isPending || update.isPending;
  const submitLabel = mode === "create" ? "Create tag" : "Save changes";

  const previewTag = {
    slug: slug || "tag",
    display_name: displayName || "Untitled tag",
    icon,
    color,
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
        <div className="bg-muted/30 flex items-center gap-3 rounded-md border p-3">
          <TagChip tag={previewTag} />
          <span className="text-muted-foreground text-xs">Live preview</span>
        </div>

        {mode === "create" ? (
          <FormField
            control={form.control}
            name="slug"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Slug</FormLabel>
                <FormControl>
                  <Input
                    placeholder="needs-review"
                    autoFocus
                    spellCheck={false}
                    className="font-mono text-sm"
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  The stable identifier used in rules, exports, and the URL.
                  Lowercase letters, digits, hyphens, and colons only.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        ) : (
          <div className="space-y-2">
            <div className="text-sm font-medium">Slug</div>
            <div className="bg-muted/30 text-muted-foreground rounded-md border px-3 py-2 font-mono text-sm">
              {tag?.slug}
              <span className="ml-2 font-sans text-xs">
                · locked after creation
              </span>
            </div>
          </div>
        )}

        <FormField
          control={form.control}
          name="display_name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Display name</FormLabel>
              <FormControl>
                <Input
                  placeholder="Needs review"
                  autoFocus={mode === "edit"}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="description"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Description</FormLabel>
              <FormControl>
                <Textarea
                  placeholder="Optional — what this tag means."
                  rows={2}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="icon"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Icon</FormLabel>
                <FormControl>
                  <IconPicker
                    value={field.value}
                    onChange={field.onChange}
                    tint={color}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="color"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Color</FormLabel>
                <FormControl>
                  <ColorPicker
                    value={field.value}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <div className="flex gap-2 border-t pt-6">
          <Button type="submit" disabled={isPending}>
            {isPending ? "Saving…" : submitLabel}
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={() => navigate({ to: "/tags" })}
          >
            Cancel
          </Button>
        </div>
      </form>
    </Form>
  );
}
