import { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  Check,
  Copy,
  Link2,
  MoreVertical,
  RefreshCw,
  Trash2,
  UserPlus,
  Users,
} from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { toast } from "sonner";
import {
  setupAccountURL,
  useCreateLoginForUser,
  useCreateUser,
  useCreateUserLogin,
  useDeleteUser,
  useDeleteUserLogin,
  useRegenerateUserLogin,
  useUserLogins,
  useUsers,
} from "@/api/queries/users";
import { withMutationToast } from "@/lib/mutation-toast";
import type { LoginAccount, User } from "@/api/types";
import { cn } from "@/lib/utils";
import { EmptyState } from "@/components/empty-state";

export function HouseholdSection() {
  const { data: users, isLoading } = useUsers();
  const [addOpen, setAddOpen] = useState(false);

  const hasMembers = !!users && users.length > 0;

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <div className="space-y-1">
          <h2 className="text-lg font-medium">Household</h2>
          <p className="text-muted-foreground text-sm">
            Add family members to track everyone's accounts in one place. Each
            member can be invited to sign in with their own login.
          </p>
        </div>
        <Dialog open={addOpen} onOpenChange={setAddOpen}>
          <DialogTrigger asChild>
            <Button size="sm" variant={hasMembers ? "outline" : "default"}>
              <UserPlus className="size-4" />
              {hasMembers ? "Add member" : "Add your first member"}
            </Button>
          </DialogTrigger>
          <AddMemberDialog onDone={() => setAddOpen(false)} />
        </Dialog>
      </div>

      {isLoading ? (
        <p className="text-muted-foreground text-sm">Loading members…</p>
      ) : !hasMembers ? (
        <EmptyState
          variant="card"
          icon={Users}
          title="No family members yet"
          description="Add members to connect their banks and attribute transactions by person."
        />
      ) : (
        <ul className="space-y-3">
          {users.map((u) => (
            <MemberRow key={u.id} user={u} />
          ))}
        </ul>
      )}
    </div>
  );
}

function MemberRow({ user }: { user: User }) {
  const { data: logins } = useUserLogins(user.id);
  const login = logins?.[0];
  const [shareToken, setShareToken] = useState<string | null>(null);
  const [createLoginOpen, setCreateLoginOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <li className="border-border bg-card rounded-lg border p-4">
      <div className="flex items-center gap-3">
        <Avatar className="size-10">
          <AvatarFallback>{initials(user.name)}</AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="truncate font-medium">{user.name}</p>
            <LoginBadge login={login} />
          </div>
          <p className="text-muted-foreground truncate text-xs">
            {user.email ?? login?.username ?? "No email"}
          </p>
        </div>
        <MemberMenu
          user={user}
          login={login}
          onCreateLogin={() => setCreateLoginOpen(true)}
          onRegenerate={(token) => setShareToken(token)}
          onDelete={() => setDeleteOpen(true)}
        />
      </div>

      <Dialog open={createLoginOpen} onOpenChange={setCreateLoginOpen}>
        <CreateLoginDialog
          user={user}
          onDone={(token) => {
            setCreateLoginOpen(false);
            if (token) setShareToken(token);
          }}
        />
      </Dialog>

      <ShareLinkDialog
        open={!!shareToken}
        token={shareToken}
        memberName={user.name}
        onClose={() => setShareToken(null)}
      />

      <DeleteMemberDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        user={user}
      />
    </li>
  );
}

function LoginBadge({ login }: { login?: LoginAccount }) {
  if (!login) {
    return (
      <Badge variant="outline" className="text-muted-foreground">
        no login
      </Badge>
    );
  }
  if (!login.has_password) {
    return (
      <Badge
        variant="secondary"
        className="border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300"
      >
        setup pending
      </Badge>
    );
  }
  return <Badge variant="secondary">{login.role}</Badge>;
}

function MemberMenu({
  user,
  login,
  onCreateLogin,
  onRegenerate,
  onDelete,
}: {
  user: User;
  login?: LoginAccount;
  onCreateLogin: () => void;
  onRegenerate: (token: string) => void;
  onDelete: () => void;
}) {
  const regenerate = useRegenerateUserLogin(user.id);
  const deleteLogin = useDeleteUserLogin(user.id);

  const onRegenerateClick = async () => {
    if (!login) return;
    try {
      const res = await regenerate.mutateAsync(login.id);
      onRegenerate(res.setup_token);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Couldn't generate setup link.",
      );
    }
  };

  const onUnlinkLogin = async () => {
    if (!login) return;
    await withMutationToast(() => deleteLogin.mutateAsync(login.id), {
      success: "Login removed.",
    });
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="text-muted-foreground hover:text-foreground size-8"
        >
          <MoreVertical className="size-4" />
          <span className="sr-only">Member actions</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        {login ? (
          <>
            {!login.has_password && (
              <DropdownMenuItem
                onSelect={(e) => {
                  e.preventDefault();
                  onRegenerateClick();
                }}
              >
                <Link2 className="size-4" />
                Get setup link
              </DropdownMenuItem>
            )}
            <DropdownMenuItem
              onSelect={(e) => {
                e.preventDefault();
                onRegenerateClick();
              }}
            >
              <RefreshCw className="size-4" />
              Reset password
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={(e) => {
                e.preventDefault();
                onUnlinkLogin();
              }}
            >
              <Trash2 className="size-4" />
              Remove login
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        ) : (
          <>
            <DropdownMenuItem
              onSelect={(e) => {
                e.preventDefault();
                onCreateLogin();
              }}
            >
              <UserPlus className="size-4" />
              Invite to sign in
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        )}
        <DropdownMenuItem
          variant="destructive"
          onSelect={(e) => {
            e.preventDefault();
            onDelete();
          }}
        >
          <Trash2 className="size-4" />
          Delete member
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

const addMemberSchema = z
  .object({
    name: z.string().trim().min(1, "Name is required").max(128),
    email: z
      .string()
      .trim()
      .email("Enter a valid email")
      .optional()
      .or(z.literal("")),
    create_login: z.boolean(),
    role: z.enum(["viewer", "editor", "admin"]),
  })
  .refine((v) => !v.create_login || (v.email && v.email.length > 0), {
    path: ["email"],
    message: "Email is required to send a login invite",
  });

type AddMemberValues = z.infer<typeof addMemberSchema>;

function AddMemberDialog({ onDone }: { onDone: () => void }) {
  const createUser = useCreateUser();
  const createLogin = useCreateLoginForUser();
  const [shareToken, setShareToken] = useState<string | null>(null);
  const [memberName, setMemberName] = useState<string>("");

  const form = useForm<AddMemberValues>({
    resolver: zodResolver(addMemberSchema),
    defaultValues: { name: "", email: "", create_login: false, role: "viewer" },
  });

  const createLoginFlag = form.watch("create_login");

  const onSubmit = async (values: AddMemberValues) => {
    try {
      const user = await createUser.mutateAsync({
        name: values.name,
        email: values.email ? values.email : undefined,
      });
      if (!values.create_login) {
        toast.success(`${user.name} added to the household.`);
        onDone();
        return;
      }
      const login = await createLogin.mutateAsync({
        userId: user.id,
        username: values.email!,
        role: values.role,
      });
      if (login.setup_token) {
        setMemberName(user.name);
        setShareToken(login.setup_token);
      } else {
        toast.success(`${user.name} added with login.`);
        onDone();
      }
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Couldn't add member.",
      );
    }
  };

  if (shareToken) {
    return (
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Share their setup link</DialogTitle>
          <DialogDescription>
            {memberName} can use this one-time link to set their password. It
            expires in 7 days.
          </DialogDescription>
        </DialogHeader>
        <SetupLinkBox token={shareToken} />
        <DialogFooter>
          <Button onClick={onDone}>Done</Button>
        </DialogFooter>
      </DialogContent>
    );
  }

  const submitting = createUser.isPending || createLogin.isPending;

  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Add a household member</DialogTitle>
        <DialogDescription>
          New members are added without a login by default. Invite them to sign
          in to share read or edit access.
        </DialogDescription>
      </DialogHeader>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Name</FormLabel>
                <FormControl>
                  <Input placeholder="e.g. Alex Canales" autoFocus {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="email"
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  Email{" "}
                  <span className="text-muted-foreground text-xs">
                    {createLoginFlag ? "(required for login)" : "(optional)"}
                  </span>
                </FormLabel>
                <FormControl>
                  <Input
                    type="email"
                    placeholder="e.g. alex@example.com"
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="create_login"
            render={({ field }) => (
              <FormItem className="border-border rounded-md border p-3">
                <div className="flex items-start gap-3">
                  <FormControl>
                    <Checkbox
                      checked={field.value}
                      onCheckedChange={(v) => field.onChange(v === true)}
                      id="create-login"
                    />
                  </FormControl>
                  <div className="flex-1 space-y-1">
                    <Label htmlFor="create-login" className="cursor-pointer">
                      Invite them to sign in
                    </Label>
                    <FormDescription>
                      We'll create a login and give you a one-time link to
                      share. They set their own password.
                    </FormDescription>
                  </div>
                </div>
              </FormItem>
            )}
          />
          {createLoginFlag && (
            <FormField
              control={form.control}
              name="role"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Role</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="viewer">
                        Viewer — sees only their own data
                      </SelectItem>
                      <SelectItem value="editor">
                        Editor — view and edit all household data
                      </SelectItem>
                      <SelectItem value="admin">
                        Admin — full access including settings
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onDone}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? "Adding…" : "Add member"}
            </Button>
          </DialogFooter>
        </form>
      </Form>
    </DialogContent>
  );
}

const createLoginSchema = z.object({
  username: z.string().trim().email("Must be a valid email"),
  role: z.enum(["viewer", "editor", "admin"]),
});

type CreateLoginValues = z.infer<typeof createLoginSchema>;

function CreateLoginDialog({
  user,
  onDone,
}: {
  user: User;
  onDone: (token: string | null) => void;
}) {
  const createLogin = useCreateUserLogin(user.id);
  const form = useForm<CreateLoginValues>({
    resolver: zodResolver(createLoginSchema),
    defaultValues: {
      username: user.email ?? "",
      role: "viewer",
    },
  });

  const onSubmit = async (values: CreateLoginValues) => {
    try {
      const login = await createLogin.mutateAsync(values);
      if (login.setup_token) {
        onDone(login.setup_token);
      } else {
        toast.success("Login created.");
        onDone(null);
      }
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Couldn't create login.",
      );
    }
  };

  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Invite {user.name} to sign in</DialogTitle>
        <DialogDescription>
          We'll create a login and give you a one-time setup link to share.
        </DialogDescription>
      </DialogHeader>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          <FormField
            control={form.control}
            name="username"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Email</FormLabel>
                <FormControl>
                  <Input type="email" autoFocus {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="role"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Role</FormLabel>
                <Select onValueChange={field.onChange} value={field.value}>
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="viewer">
                      Viewer — sees only their own data
                    </SelectItem>
                    <SelectItem value="editor">
                      Editor — view and edit all household data
                    </SelectItem>
                    <SelectItem value="admin">
                      Admin — full access including settings
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onDone(null)}
              disabled={createLogin.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={createLogin.isPending}>
              {createLogin.isPending ? "Creating…" : "Create login"}
            </Button>
          </DialogFooter>
        </form>
      </Form>
    </DialogContent>
  );
}

function ShareLinkDialog({
  open,
  token,
  memberName,
  onClose,
}: {
  open: boolean;
  token: string | null;
  memberName: string;
  onClose: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Share their setup link</DialogTitle>
          <DialogDescription>
            {memberName} can use this one-time link to set their password. It
            expires in 7 days.
          </DialogDescription>
        </DialogHeader>
        {token && <SetupLinkBox token={token} />}
        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SetupLinkBox({ token }: { token: string }) {
  const url = useMemo(() => setupAccountURL(token), [token]);
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Couldn't copy. Select the link and copy manually.");
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <Input
          value={url}
          readOnly
          className="bg-muted/40 font-mono text-xs"
          onFocus={(e) => e.currentTarget.select()}
        />
        <Button
          type="button"
          variant={copied ? "default" : "outline"}
          size="sm"
          onClick={copy}
          className={cn(copied && "bg-emerald-600 hover:bg-emerald-600/90")}
        >
          {copied ? (
            <>
              <Check className="size-4" />
              Copied
            </>
          ) : (
            <>
              <Copy className="size-4" />
              Copy
            </>
          )}
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        Anyone with this link can set the password. Share it directly with the
        person and don't post it anywhere public.
      </p>
    </div>
  );
}

function DeleteMemberDialog({
  open,
  onOpenChange,
  user,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: User;
}) {
  const deleteUser = useDeleteUser();

  const onConfirm = async () => {
    const ok = await withMutationToast(() => deleteUser.mutateAsync(user.id), {
      success: `${user.name} removed.`,
      error:
        "Couldn't remove this member. They may still have bank connections attached — disconnect those first.",
    });
    if (ok) onOpenChange(false);
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {user.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            This deletes the household member and their login (if any). Bank
            connections attached to them must be disconnected first. This can't
            be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={(e) => {
              e.preventDefault();
              onConfirm();
            }}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Remove member
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean).slice(0, 2);
  if (parts.length === 0) return "?";
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("");
}
