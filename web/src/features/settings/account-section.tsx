import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { useMe } from "@/api/queries/me";
import { useChangePassword } from "@/api/queries/account";
import { withMutationToast } from "@/lib/mutation-toast";
import { SettingsSectionHeader } from "@/components/settings-section-header";
import { LeaveGuard } from "@/components/leave-guard";

const schema = z
  .object({
    current_password: z.string().min(1, "Current password is required"),
    new_password: z.string().min(8, "Must be at least 8 characters"),
    confirm_password: z.string().min(1, "Confirm your new password"),
  })
  .refine((v) => v.new_password === v.confirm_password, {
    path: ["confirm_password"],
    message: "Passwords do not match",
  });

type Values = z.infer<typeof schema>;

export function AccountSection() {
  const { data: me } = useMe();
  const changePassword = useChangePassword();

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { current_password: "", new_password: "", confirm_password: "" },
  });

  const onSubmit = async (values: Values) => {
    const ok = await withMutationToast(
      () => changePassword.mutateAsync(values),
      { success: "Password updated." },
    );
    if (ok) form.reset();
  };

  return (
    <div className="space-y-6">
      <SettingsSectionHeader
        title="Account"
        description="Your sign-in identity. The admin role is managed by the household owner."
      />

      <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label="Email" value={me?.username ?? "—"} />
        <Field label="Role" value={me?.role ?? "—"} mono />
      </dl>

      <div className="border-border space-y-4 border-t pt-6">
        <SettingsSectionHeader
          level="sub"
          title="Change password"
          description="At least 8 characters. You'll stay signed in."
        />
        <Form {...form}>
          <LeaveGuard
            when={form.formState.isDirty && !form.formState.isSubmitting}
            title="Discard your password change?"
            description="You've started typing a new password but haven't saved it. Leaving will lose what you typed."
            confirmLabel="Discard"
          />
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="current_password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Current password</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      autoComplete="current-password"
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="new_password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>New password</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      autoComplete="new-password"
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="confirm_password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Confirm new password</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      autoComplete="new-password"
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button type="submit" disabled={changePassword.isPending}>
              {changePassword.isPending && <Loader2 className="size-4 animate-spin" />}
              {changePassword.isPending ? "Updating…" : "Update password"}
            </Button>
          </form>
        </Form>
      </div>
    </div>
  );
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs uppercase tracking-wider">{label}</dt>
      <dd className={mono ? "font-mono text-sm" : "text-sm"}>{value}</dd>
    </div>
  );
}
