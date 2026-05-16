import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { CheckCircle2, ShieldAlert } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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

  return (
    <div className="bg-muted/30 flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        {info.isLoading ? (
          <>
            <CardHeader>
              <CardTitle>Checking your link…</CardTitle>
              <CardDescription>One moment.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="bg-muted h-9 animate-pulse rounded-md" />
            </CardContent>
          </>
        ) : info.error instanceof ApiError &&
          info.error.code === "ALREADY_SETUP" ? (
          <>
            <CardHeader>
              <div className="bg-emerald-500/10 mb-2 flex size-10 items-center justify-center rounded-xl">
                <CheckCircle2 className="size-5 text-emerald-600 dark:text-emerald-400" />
              </div>
              <CardTitle>This account is already set up</CardTitle>
              <CardDescription>
                You've already chosen a password. Sign in below.
              </CardDescription>
            </CardHeader>
            <CardFooter>
              <Button onClick={() => navigate({ to: "/login" })} className="w-full">
                Go to sign in
              </Button>
            </CardFooter>
          </>
        ) : info.error ? (
          <>
            <CardHeader>
              <div className="bg-destructive/10 mb-2 flex size-10 items-center justify-center rounded-xl">
                <ShieldAlert className="text-destructive size-5" />
              </div>
              <CardTitle>This setup link is invalid</CardTitle>
              <CardDescription>
                {info.error instanceof ApiError
                  ? info.error.message
                  : "The link is invalid or has expired."}
                {" "}
                Ask the person who invited you to send a fresh link.
              </CardDescription>
            </CardHeader>
          </>
        ) : (
          <>
            <CardHeader>
              <CardTitle>Set your password</CardTitle>
              <CardDescription>
                Welcome,{" "}
                <span className="text-foreground font-medium">
                  {info.data?.username}
                </span>
                . Choose a password to finish setting up your Breadbox account.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Form {...form}>
                <form
                  onSubmit={form.handleSubmit(onSubmit)}
                  className="space-y-4"
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
                            autoComplete="new-password"
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <Button type="submit" className="w-full" disabled={submitting}>
                    {submitting ? "Setting password…" : "Set password & sign in"}
                  </Button>
                </form>
              </Form>
            </CardContent>
          </>
        )}
      </Card>
    </div>
  );
}
