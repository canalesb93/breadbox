import { useForm, type Resolver, type SubmitHandler } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNavigate } from "@tanstack/react-router";
import {
  Bot,
  Loader2,
  ShieldCheck,
  ShieldOff,
  User,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "@/api/client";
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { FormFooter } from "@/components/form-footer";
import { LeaveGuard } from "@/components/leave-guard";
import { cn } from "@/lib/utils";
import { useCreateAPIKey } from "@/api/queries/api-keys";
import { storePendingPlaintextKey } from "@/features/api-keys/plaintext-store";

const formSchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .max(120, "Keep it under 120 characters"),
  scope: z.enum(["full_access", "read_only"]),
  actor_type: z.enum(["agent", "user", "system"]),
  actor_name: z
    .string()
    .max(120, "Keep it under 120 characters")
    .optional()
    .or(z.literal("")),
});

type APIKeyFormValues = z.infer<typeof formSchema>;

interface ScopeOption {
  value: APIKeyFormValues["scope"];
  label: string;
  description: string;
  Icon: LucideIcon;
}

const SCOPE_OPTIONS: ScopeOption[] = [
  {
    value: "full_access",
    label: "Full access",
    description: "Read everything, write everything. Triggers syncs, edits transactions.",
    Icon: ShieldCheck,
  },
  {
    value: "read_only",
    label: "Read only",
    description: "Query data only. Cannot categorize, sync, or use write MCP tools.",
    Icon: ShieldOff,
  },
];

interface ActorOption {
  value: APIKeyFormValues["actor_type"];
  label: string;
  description: string;
  Icon: LucideIcon;
}

const ACTOR_OPTIONS: ActorOption[] = [
  {
    value: "agent",
    label: "Agent",
    description: "AI assistants, Claude integrations, MCP clients.",
    Icon: Bot,
  },
  {
    value: "user",
    label: "User",
    description: "A human-driven script, CLI, or one-off integration.",
    Icon: User,
  },
  {
    value: "system",
    label: "System",
    description: "Internal automation — schedulers, jobs, infra plumbing.",
    Icon: ShieldCheck,
  },
];

export function APIKeyForm() {
  const navigate = useNavigate();
  const create = useCreateAPIKey();

  const form = useForm<APIKeyFormValues>({
    resolver: zodResolver(formSchema) as Resolver<APIKeyFormValues>,
    defaultValues: {
      name: "",
      scope: "full_access",
      actor_type: "agent",
      actor_name: "",
    },
  });

  const onSubmit: SubmitHandler<APIKeyFormValues> = async (values) => {
    try {
      const created = await create.mutateAsync({
        name: values.name,
        scope: values.scope,
        actor_type: values.actor_type,
        actor_name: values.actor_name || undefined,
      });
      storePendingPlaintextKey({
        id: created.id,
        name: created.name,
        plaintext: created.plaintext_key,
      });
      toast.success(`Created "${values.name}".`);
      // Reset before navigating so LeaveGuard doesn't intercept the
      // post-save nav to /api-keys/created. Only on success.
      form.reset(values);
      navigate({ to: "/api-keys/created" });
    } catch (err) {
      const msg =
        err instanceof ApiError ? err.message : "Couldn't create the key.";
      toast.error(msg);
    }
  };

  return (
    <Form {...form}>
      <LeaveGuard
        when={form.formState.isDirty && !form.formState.isSubmitting}
      />
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-8">
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Name</FormLabel>
              <FormControl>
                <Input
                  placeholder="e.g. Claude MCP, Mobile CLI"
                  autoFocus
                  autoComplete="off"
                  autoCorrect="off"
                  spellCheck={false}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                A descriptive name so you recognise this key in the audit log.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="scope"
          render={({ field }) => (
            <FormItem className="space-y-3">
              <FormLabel>Scope</FormLabel>
              <FormControl>
                <RadioGroup
                  value={field.value}
                  onValueChange={field.onChange}
                  className="grid gap-3 sm:grid-cols-2"
                >
                  {SCOPE_OPTIONS.map((option) => (
                    <OptionCard
                      key={option.value}
                      value={option.value}
                      selected={field.value === option.value}
                      label={option.label}
                      description={option.description}
                      Icon={option.Icon}
                    />
                  ))}
                </RadioGroup>
              </FormControl>
              <FormDescription>
                Scope is fixed at mint — to change it, revoke this key and
                create a new one.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="actor_type"
          render={({ field }) => (
            <FormItem className="space-y-3">
              <FormLabel>Actor type</FormLabel>
              <FormControl>
                <RadioGroup
                  value={field.value}
                  onValueChange={field.onChange}
                  className="grid gap-3 sm:grid-cols-3"
                >
                  {ACTOR_OPTIONS.map((option) => (
                    <OptionCard
                      key={option.value}
                      value={option.value}
                      selected={field.value === option.value}
                      label={option.label}
                      description={option.description}
                      Icon={option.Icon}
                    />
                  ))}
                </RadioGroup>
              </FormControl>
              <FormDescription>
                Drives attribution on every action this key takes — visible in
                the activity timeline.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="actor_name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                Actor display name{" "}
                <span className="text-muted-foreground font-normal">
                  · optional
                </span>
              </FormLabel>
              <FormControl>
                <Input
                  placeholder="Defaults to the key name"
                  autoComplete="off"
                  autoCorrect="off"
                  spellCheck={false}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                Override the display name shown in the activity timeline.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormFooter
          secondary={
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => navigate({ to: "/api-keys" })}
              disabled={create.isPending}
            >
              Cancel
            </Button>
          }
          primary={
            <Button type="submit" size="sm" disabled={create.isPending}>
              {create.isPending && <Loader2 className="size-4 animate-spin" />}
              {create.isPending ? "Creating…" : "Create key"}
            </Button>
          }
        />
      </form>
    </Form>
  );
}

interface OptionCardProps {
  value: string;
  selected: boolean;
  label: string;
  description: string;
  Icon: LucideIcon;
}

// OptionCard is a wrapped RadioGroupItem styled as a tappable tile —
// the indicator stays in the corner; the whole card is the hit target.
function OptionCard({
  value,
  selected,
  label,
  description,
  Icon,
}: OptionCardProps) {
  const id = `option-${value}`;
  return (
    <label
      htmlFor={id}
      className={cn(
        "group relative flex cursor-pointer flex-col gap-2 rounded-lg border p-4 transition-colors",
        "hover:border-foreground/30",
        selected
          ? "border-primary bg-primary/[0.04] ring-primary/30 ring-1"
          : "border-border bg-card",
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <Icon
          className={cn(
            "size-4 transition-colors",
            selected ? "text-primary" : "text-muted-foreground",
          )}
        />
        <RadioGroupItem id={id} value={value} className="mt-0.5" />
      </div>
      <div className="space-y-1">
        <div className="text-sm font-medium">{label}</div>
        <p className="text-muted-foreground text-xs leading-relaxed">
          {description}
        </p>
      </div>
    </label>
  );
}
