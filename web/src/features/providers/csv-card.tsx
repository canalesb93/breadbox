import { Link } from "@tanstack/react-router";
import { FileSpreadsheet, Upload } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { ProviderHealthResponse } from "@/api/types";
import { ProviderStats } from "./provider-status";

interface CsvCardProps {
  health: ProviderHealthResponse | undefined;
}

export function CsvCard({ health }: CsvCardProps) {
  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b">
        <div className="flex items-center gap-3">
          <div className="bg-amber-500/10 text-amber-600 dark:text-amber-400 flex size-10 items-center justify-center rounded-lg">
            <FileSpreadsheet className="size-5" />
          </div>
          <div className="min-w-0 flex-1">
            <CardTitle className="text-base">CSV import</CardTitle>
            <CardDescription className="text-xs">
              Drop in transactions exported from any bank — no API credentials required.
            </CardDescription>
          </div>
          <Badge variant="outline" className="border-emerald-500/40 text-emerald-600 dark:text-emerald-400">
            Always available
          </Badge>
        </div>
      </CardHeader>

      <CardContent className="space-y-6 pt-2">
        <ProviderStats health={health} />
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-muted-foreground max-w-md text-sm">
            Useful when a bank isn't supported by Plaid or Teller, or as a one-time backfill for historical transactions.
          </p>
          <Button asChild size="sm">
            <Link to="/connections" search={{ action: "connect" }}>
              <Upload className="size-3.5" />
              Import CSV
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
