import { useMemo, useState } from "react";
import {
  useForm,
  useWatch,
  type Resolver,
  type SubmitHandler,
} from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Check, ChevronsUpDown, X } from "lucide-react";
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
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { IconPicker } from "@/components/icon-picker";
import { ColorPicker } from "@/components/color-picker";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { DynamicIcon } from "@/lib/icon";
import {
  useCategories,
  useCreateCategory,
  useUpdateCategory,
} from "@/api/queries/categories";
import { withMutationToast } from "@/lib/mutation-toast";
import type { Category } from "@/api/types";

const baseSchema = z.object({
  display_name: z
    .string()
    .min(1, "Display name is required")
    .max(128, "Keep it under 128 characters"),
  parent_id: z.string().nullable(),
  icon: z.string().nullable(),
  color: z.string().nullable(),
  sort_order: z.number().int(),
  hidden: z.boolean(),
});

export type CategoryFormValues = z.infer<typeof baseSchema>;

interface CategoryFormProps {
  mode: "create" | "edit";
  category?: Category;
}

export function CategoryForm({ mode, category }: CategoryFormProps) {
  const navigate = useNavigate();
  const create = useCreateCategory();
  const update = useUpdateCategory();

  const form = useForm<CategoryFormValues>({
    resolver: zodResolver(baseSchema) as Resolver<CategoryFormValues>,
    defaultValues: {
      display_name: category?.display_name ?? "",
      parent_id: category?.parent_id ?? null,
      icon: category?.icon ?? null,
      color: category?.color ?? null,
      sort_order: category?.sort_order ?? 0,
      hidden: category?.hidden ?? false,
    },
  });

  const [icon, color, displayName, parentId] = useWatch({
    control: form.control,
    name: ["icon", "color", "display_name", "parent_id"],
  });

  const onSubmit: SubmitHandler<CategoryFormValues> = async (values) => {
    if (mode === "create") {
      const ok = await withMutationToast(
        () =>
          create.mutateAsync({
            display_name: values.display_name,
            parent_id: values.parent_id ?? null,
            icon: values.icon ?? null,
            color: values.color ?? null,
            sort_order: values.sort_order ?? 0,
          }),
        { success: "Category created." },
      );
      if (ok) navigate({ to: "/categories" });
    } else if (category) {
      const ok = await withMutationToast(
        () =>
          update.mutateAsync({
            id: category.short_id,
            input: {
              display_name: values.display_name,
              icon: values.icon ?? null,
              color: values.color ?? null,
              sort_order: values.sort_order,
              hidden: values.hidden,
            },
          }),
        { success: "Category updated." },
      );
      if (ok) navigate({ to: "/categories" });
    }
  };

  const isPending = create.isPending || update.isPending;
  const submitLabel = mode === "create" ? "Create category" : "Save changes";

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
        <div className="bg-muted/30 flex items-center gap-3 rounded-md border p-3">
          <CategoryIconTile icon={icon} color={color} size="lg" />
          <div className="min-w-0">
            <div className="truncate font-medium">
              {displayName || "Untitled category"}
            </div>
            <div className="text-muted-foreground text-xs">
              {category?.parent_display_name ??
                (mode === "create" && parentId
                  ? "Sub-category"
                  : "Top-level category")}
            </div>
          </div>
        </div>

        <FormField
          control={form.control}
          name="display_name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Display name</FormLabel>
              <FormControl>
                <Input
                  placeholder="e.g. Coffee shops"
                  autoFocus={mode === "create"}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {mode === "create" && (
          <FormField
            control={form.control}
            name="parent_id"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Parent category</FormLabel>
                <FormControl>
                  <ParentCategoryPicker
                    value={field.value ?? null}
                    onChange={(id) => field.onChange(id)}
                  />
                </FormControl>
                <FormDescription>
                  Pick a parent to make this a sub-category, or leave empty
                  for a new top-level group. Parent is locked once the
                  category exists.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        )}

        {mode === "edit" && category?.parent_display_name && (
          <div className="space-y-2">
            <div className="text-sm font-medium">Parent category</div>
            <div className="bg-muted/30 text-muted-foreground rounded-md border px-3 py-2 text-sm">
              {category.parent_display_name}
              <span className="text-muted-foreground/60 ml-2 text-xs">
                · locked after creation
              </span>
            </div>
          </div>
        )}

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="icon"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Icon</FormLabel>
                <FormControl>
                  <IconPicker
                    value={field.value ?? null}
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
                    value={field.value ?? null}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <FormField
          control={form.control}
          name="sort_order"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Sort order</FormLabel>
              <FormControl>
                <Input
                  type="number"
                  inputMode="numeric"
                  {...field}
                  value={field.value ?? 0}
                />
              </FormControl>
              <FormDescription>
                Lower numbers appear first in pickers and lists.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        {mode === "edit" && (
          <FormField
            control={form.control}
            name="hidden"
            render={({ field }) => (
              <FormItem className="bg-muted/30 flex flex-row items-start gap-3 rounded-md border p-3">
                <FormControl>
                  <Checkbox
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <div className="space-y-1 leading-none">
                  <FormLabel>Hide this category</FormLabel>
                  <FormDescription>
                    Hidden categories don't appear in pickers, but stay
                    attached to their existing transactions.
                  </FormDescription>
                </div>
              </FormItem>
            )}
          />
        )}

        <div className="flex gap-2 border-t pt-6">
          <Button type="submit" disabled={isPending}>
            {isPending ? "Saving…" : submitLabel}
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={() => navigate({ to: "/categories" })}
          >
            Cancel
          </Button>
        </div>
      </form>
    </Form>
  );
}

function ParentCategoryPicker({
  value,
  onChange,
}: {
  value: string | null;
  onChange: (id: string | null) => void;
}) {
  const [open, setOpen] = useState(false);
  const { data: tree } = useCategories();
  // Only top-level, non-system, non-hidden categories can be parents.
  const candidates = useMemo(
    () => (tree ?? []).filter((c) => !c.parent_id && !c.is_system && !c.hidden),
    [tree],
  );
  const current = candidates.find((c) => c.id === value);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="h-9 w-full justify-between gap-2 px-2.5"
        >
          {current ? (
            <span className="flex items-center gap-2 truncate">
              <DynamicIcon
                name={current.icon}
                className="size-4"
                style={current.color ? { color: current.color } : undefined}
              />
              <span className="truncate">{current.display_name}</span>
            </span>
          ) : (
            <span className="text-muted-foreground text-sm">
              Top-level (no parent)
            </span>
          )}
          <ChevronsUpDown className="size-4 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
        <Command>
          <CommandInput placeholder="Search categories…" />
          <CommandList>
            <CommandEmpty>No categories found.</CommandEmpty>
            <CommandItem
              value="top-level no parent"
              onSelect={() => {
                onChange(null);
                setOpen(false);
              }}
              className="text-muted-foreground"
            >
              <X className="size-4" />
              Top-level (no parent)
              {!value && <Check className="ml-auto size-4" />}
            </CommandItem>
            {candidates.map((c) => (
              <CommandItem
                key={c.id}
                value={c.display_name}
                onSelect={() => {
                  onChange(c.id);
                  setOpen(false);
                }}
              >
                <DynamicIcon
                  name={c.icon}
                  className="size-4"
                  style={c.color ? { color: c.color } : undefined}
                />
                {c.display_name}
                {c.id === value && <Check className="ml-auto size-4" />}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

