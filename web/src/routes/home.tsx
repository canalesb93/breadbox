import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useMe } from "@/api/queries/me";

export function HomePage() {
  const { data: me } = useMe();

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {me ? `Welcome, ${me.username}` : "Welcome"}
        </h1>
        <p className="text-muted-foreground mt-1 text-sm">
          You're using the v2 admin preview. Real dashboard data lands in the next PR.
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardDescription>Tech stack</CardDescription>
            <CardTitle className="text-base font-medium">
              React · TypeScript · Vite · TanStack · shadcn/ui
            </CardTitle>
          </CardHeader>
          <CardContent className="text-muted-foreground text-sm">
            Single binary in production. Old admin UI stays at <code>/</code>.
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardDescription>API split</CardDescription>
            <CardTitle className="text-base font-medium">
              <code>/api/v1/*</code> public · <code>/web/v1/*</code> internal
            </CardTitle>
          </CardHeader>
          <CardContent className="text-muted-foreground text-sm">
            Session-cookie auth on <code>/web/v1/*</code>. Zero stability promise.
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardDescription>Status</CardDescription>
            <CardTitle className="text-base font-medium">Phase 0 · shell</CardTitle>
          </CardHeader>
          <CardContent className="text-muted-foreground text-sm">
            Navigable scaffold. Pages built one at a time per weekend.
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
