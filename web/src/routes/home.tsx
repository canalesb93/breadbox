import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { PageHeader } from "@/components/page-header";
import { useMe } from "@/api/queries/me";

export function HomePage() {
  const { data: me } = useMe();

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        eyebrow="Overview"
        title={me ? `Welcome, ${me.username}` : "Welcome"}
        description="You're using the v2 admin preview. Real dashboard data lands in the next PR."
      />

      <div className="grid gap-4 md:grid-cols-3">
        <Card className="gap-3 py-5">
          <CardHeader className="px-5">
            <CardDescription className="text-[11px] font-medium tracking-[0.06em] uppercase">
              Tech stack
            </CardDescription>
            <CardTitle className="text-sm leading-snug font-medium">
              React · TypeScript · Vite · TanStack · shadcn/ui
            </CardTitle>
          </CardHeader>
          <CardContent className="text-muted-foreground px-5 text-sm">
            Single binary in production. Old admin UI stays at <code>/</code>.
          </CardContent>
        </Card>

        <Card className="gap-3 py-5">
          <CardHeader className="px-5">
            <CardDescription className="text-[11px] font-medium tracking-[0.06em] uppercase">
              API split
            </CardDescription>
            <CardTitle className="text-sm leading-snug font-medium">
              <code>/api/v1/*</code> public · <code>/web/v1/*</code> internal
            </CardTitle>
          </CardHeader>
          <CardContent className="text-muted-foreground px-5 text-sm">
            Session-cookie auth on <code>/web/v1/*</code>. Zero stability promise.
          </CardContent>
        </Card>

        <Card className="gap-3 py-5">
          <CardHeader className="px-5">
            <CardDescription className="text-[11px] font-medium tracking-[0.06em] uppercase">
              Status
            </CardDescription>
            <CardTitle className="text-sm leading-snug font-medium">
              Phase 0 · shell
            </CardTitle>
          </CardHeader>
          <CardContent className="text-muted-foreground px-5 text-sm">
            Navigable scaffold. Pages built one at a time per weekend.
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
