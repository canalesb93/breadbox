import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowRight, CheckCircle2, Loader2, ShieldAlert, UserPlus } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { AuthShell } from "@/components/auth-shell";
import { StatusPanel } from "@/components/status-panel";
import { toast } from "sonner";
import { useSetupAccount, useSetupAccountInfo } from "@/api/queries/auth";
import { ApiError } from "@/api/client";

const setupSchema = z
  .object({
    password: z.string().min(8, "Must be at least 8 characters"),
    confirm_password: z.string().min(1, "Confirm your password"),
  })
  .refine((v) => v.password === v.confirm_password, {
    path: ["confirm_password"],
    message: "Passwords do not match",
  });

type SetupValues = z.infer<typeof setupSchema>;

export function SetupAccountPage() {
  const navigate = useNavigate();
  const { token } = useParams({ strict: false }) as { token?: string };
  const safeToken = token ?? "";
  const info = useSetupAccountInfo(safeToken);
  const submit = useSetupAccount(safeToken);
  const [submitting, setSubmitting] = useState(false);

  const form = useForm<SetupValues>({
    resolver: zodResolver(setupSchema),
    defaultValues: { password: "", confirm_password: "" },
  });

  const onSubmit = async (values: SetupValues) => {
    setSubmitting(true);
    try {
      await submit.mutateAsync(values);
      toast.success("Password set. Welcome to Breadbox.");
      navigate({ to: "/" });
    } catch (err) {
      const msg =
        err instanceof ApiError ? err.message : "Something went wrong. Try again.";
      toast.error(msg);
    } finally {
      setSubmitting(false);
    }
  };

  if (info.isLoading) {
    return (
      <AuthShell
        eyebrow={
          <>
            <Loader2 className="size-3 animate-spin" />
            Verifying invitation
          </>
        }
        title="Checking your link…"
        description="One moment while we validate this setup token."
      >
        <div className="flex flex-col gap-4">
          <Skeleton className="h-9 w-full rounded-md" />
          <Skeleton className="h-9 w-full rounded-md" />
          <Skeleton className="h-9 w-full rounded-md" />
        </div>
      </AuthShell>
    );
  }

  if (info.error instanceof ApiError && info.error.code === "ALREADY_SETUP") {
    return (
      <AuthShell
        eyebrow="Already set up"
        title="This account is already set up"
        description="You've already chosen a password for this Breadbox account. Sign in below to continue."
      >
        <StatusPanel
          tone="success"
          icon={CheckCircle2}
          heading="Setup complete"
          body="Your password is already on file. The setup link won't be valid a second time."
        />
        <Button
          onClick={() => navigate({ to: "/login" })}
          className="mt-5 w-full"
        >
          Go to sign in
          <ArrowRight className="size-4" />
        </Button>
      </AuthShell>
    );
  }

  if (info.error) {
    return (
      <AuthShell
        eyebrow="Invalid link"
        title="This setup link won't work"
        description={
          info.error instanceof ApiError
            ? info.error.message
            : "The link is invalid or has expired."
        }
      >
        <StatusPanel
          tone="destructive"
          icon={ShieldAlert}
          heading="Setup link expired or invalid"
          body="Ask the person who invited you to send a fresh link from their household admin dashboard. Existing links can be regenerated from Settings → Accounts."
        />
      </AuthShell>
    );
  }

  return (
    <AuthShell
      eyebrow={
        <>
          <UserPlus className="size-3" />
          Finish setup
        </>
      }
      title="Set your password"
      description={
        <>
          Welcome,{" "}
          <span className="text-foreground font-medium">
            {info.data?.username}
          </span>
          . Choose a password to finish setting up your Breadbox account.
        </>
      }
      footer={
        <span>
          Passwords are hashed with bcrypt and never leave your server.
        </span>
      }
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className="flex flex-col gap-5"
          noValidate
        >
          <FormField
            control={form.control}
            name="password"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Password</FormLabel>
                <FormControl>
                  <Input
                    type="password"
                    placeholder="At least 8 characters"
                    autoComplete="new-password"
                    autoFocus
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
                <FormLabel>Confirm password</FormLabel>
                <FormControl>
                  <Input
                    type="password"
                    placeholder="Re-enter your password"
                    autoComplete="new-password"
                    enterKeyHint="go"
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button type="submit" className="w-full" disabled={submitting}>
            {submitting ? (
              <>
                <Loader2 className="size-4 animate-spin" />
                Setting password…
              </>
            ) : (
              <>
                Set password & sign in
                <ArrowRight className="size-4" />
              </>
            )}
          </Button>
        </form>
      </Form>
    </AuthShell>
  );
}

