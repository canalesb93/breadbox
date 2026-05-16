import { useEffect, useMemo, useRef, useState } from "react";
import { FileSpreadsheet, Loader2, Upload, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { toast } from "sonner";
import { ApiError } from "@/api/client";
import {
  useCsvImport,
  useCsvPreview,
  type CsvImportInput,
} from "@/api/queries/connections";
import type { CsvColumnKey, CsvPreviewResult } from "@/api/types";
import { cn } from "@/lib/utils";

// Cap CSV uploads on the client side. The backend tolerates up to 50MB but
// 10MB matches the v1 admin behaviour and gives the user instant local
// feedback before the network round-trip.
const MAX_CSV_BYTES = 10 * 1024 * 1024;

// CsvImportForm is the CSV branch of the Connect-bank Sheet. It owns the
// upload → mapping → import flow inside whichever Sheet renders it (the
// Connect-bank Sheet on the connections list page, or the same Sheet
// inlined on the detail page in append mode).
export interface CsvImportFormProps {
  // Pre-target an existing CSV connection. When set, the form skips the
  // family-member + account-name fields and POSTs `connection_id` so the
  // backend appends rows to that connection.
  appendToConnectionId?: string;
  // Pre-selected family-member UUID for new connections. Ignored when
  // `appendToConnectionId` is set.
  userId?: string;
  onSuccess: (result: {
    connection_id: string;
    imported: number;
    appended: boolean;
  }) => void;
  onCancel: () => void;
}

type Stage =
  | { kind: "drop" }
  | {
      kind: "map";
      file: File;
      preview: CsvPreviewResult;
    };

export function CsvImportForm({
  appendToConnectionId,
  userId,
  onSuccess,
  onCancel,
}: CsvImportFormProps) {
  const [stage, setStage] = useState<Stage>({ kind: "drop" });
  const previewMut = useCsvPreview();
  const importMut = useCsvImport();

  async function onFilePicked(file: File | null) {
    if (!file) return;
    if (file.size > MAX_CSV_BYTES) {
      toast.error(
        `File is too large. Keep CSVs under ${Math.floor(MAX_CSV_BYTES / 1024 / 1024)} MB.`,
      );
      return;
    }
    try {
      const preview = await previewMut.mutateAsync(file);
      setStage({ kind: "map", file, preview });
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : "Couldn't parse that CSV. Try a different file.";
      toast.error(msg);
    }
  }

  async function onImport(mapping: CsvImportInput) {
    try {
      const result = await importMut.mutateAsync(mapping);
      onSuccess({
        connection_id: result.connection_id,
        imported: result.imported_transactions,
        appended: !!appendToConnectionId,
      });
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : "Couldn't import that CSV. Try again.";
      toast.error(msg);
    }
  }

  if (stage.kind === "drop") {
    return (
      <CsvDropStage
        onFile={onFilePicked}
        loading={previewMut.isPending}
        onCancel={onCancel}
      />
    );
  }

  return (
    <CsvMapStage
      file={stage.file}
      preview={stage.preview}
      onBack={() => {
        previewMut.reset();
        setStage({ kind: "drop" });
      }}
      onImport={onImport}
      importing={importMut.isPending}
      appendToConnectionId={appendToConnectionId}
      userId={userId}
    />
  );
}

// -- Stage 1: drop / browse -------------------------------------------------

function CsvDropStage({
  onFile,
  loading,
  onCancel,
}: {
  onFile: (file: File | null) => void;
  loading: boolean;
  onCancel: () => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);

  return (
    <div className="flex flex-1 flex-col gap-4 overflow-y-auto">
      <label
        htmlFor="csv-file"
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragOver(false);
          if (loading) return;
          const file = e.dataTransfer.files[0];
          if (file) onFile(file);
        }}
        className={cn(
          "border-border/60 hover:border-primary/40 hover:bg-accent/30 flex min-h-[180px] cursor-pointer flex-col items-center justify-center gap-3 rounded-lg border-2 border-dashed p-6 text-center transition",
          dragOver && "border-primary bg-accent/40",
          loading && "pointer-events-none opacity-60",
        )}
      >
        <input
          id="csv-file"
          ref={inputRef}
          type="file"
          accept=".csv,text/csv"
          className="hidden"
          onChange={(e) => onFile(e.target.files?.[0] ?? null)}
          disabled={loading}
        />
        {loading ? (
          <Loader2 className="text-muted-foreground size-8 animate-spin" />
        ) : (
          <Upload className="text-muted-foreground size-8" />
        )}
        <div>
          <div className="text-sm font-medium">
            {loading ? "Parsing CSV…" : "Drop a CSV here, or click to browse"}
          </div>
          <p className="text-muted-foreground mt-1 text-xs">
            Up to {Math.floor(MAX_CSV_BYTES / 1024 / 1024)} MB. We'll detect
            columns automatically — you can adjust them on the next step.
          </p>
        </div>
      </label>

      <div className="text-muted-foreground flex items-start gap-2 rounded-md border bg-muted/30 p-3 text-xs">
        <FileSpreadsheet className="text-muted-foreground/80 mt-0.5 size-4 shrink-0" />
        <span>
          Header row required. Common bank exports (Chase, Capital One, Amex,
          BofA) are auto-detected.
        </span>
      </div>

      <div className="mt-auto flex justify-end gap-2 pt-2">
        <Button variant="outline" onClick={onCancel} disabled={loading}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

// -- Stage 2: map columns + import -----------------------------------------

const REQUIRED_KEYS: CsvColumnKey[] = ["date", "description"];
// `amount` is required when has_debit_credit is false. When true, debit +
// credit are required instead.
const OPTIONAL_KEYS: CsvColumnKey[] = ["category", "merchant_name"];

const COLUMN_LABELS: Record<CsvColumnKey, string> = {
  date: "Date",
  description: "Description",
  amount: "Amount",
  debit: "Debit",
  credit: "Credit",
  category: "Category",
  merchant_name: "Merchant",
};

function CsvMapStage({
  file,
  preview,
  onBack,
  onImport,
  importing,
  appendToConnectionId,
  userId,
}: {
  file: File;
  preview: CsvPreviewResult;
  onBack: () => void;
  onImport: (input: CsvImportInput) => void;
  importing: boolean;
  appendToConnectionId?: string;
  userId?: string;
}) {
  const [hasDebitCredit, setHasDebitCredit] = useState<boolean>(
    !!preview.has_debit_credit,
  );
  const [positiveIsDebit, setPositiveIsDebit] = useState<boolean>(
    !!preview.positive_is_debit,
  );
  const [accountName, setAccountName] = useState("CSV Import");

  // Mapping state — pre-fill from the inferred mapping. Map "no selection"
  // as -1 to mirror the v1 admin convention.
  const [mapping, setMapping] = useState<Record<CsvColumnKey, number>>(() => {
    const seed: Record<CsvColumnKey, number> = {
      date: -1,
      description: -1,
      amount: -1,
      debit: -1,
      credit: -1,
      category: -1,
      merchant_name: -1,
    };
    for (const k of Object.keys(preview.inferred_mapping) as CsvColumnKey[]) {
      const v = preview.inferred_mapping[k];
      if (typeof v === "number") seed[k] = v;
    }
    return seed;
  });

  // Keep the debit/credit toggle's defaults consistent with the inferred
  // mapping if the toggle changes. We don't clear amount; the backend
  // ignores it when has_debit_credit is true.
  useEffect(() => {
    if (hasDebitCredit && (mapping.debit === -1 || mapping.credit === -1)) {
      // Best-effort: if the inferred mapping had debit/credit, restore them.
      const inferredDebit = preview.inferred_mapping.debit;
      const inferredCredit = preview.inferred_mapping.credit;
      setMapping((m) => ({
        ...m,
        debit: m.debit === -1 && typeof inferredDebit === "number" ? inferredDebit : m.debit,
        credit: m.credit === -1 && typeof inferredCredit === "number" ? inferredCredit : m.credit,
      }));
    }
  }, [hasDebitCredit, preview.inferred_mapping, mapping.debit, mapping.credit]);

  function setColumn(key: CsvColumnKey, raw: string) {
    const v = raw === "none" ? -1 : Number(raw);
    setMapping((m) => ({ ...m, [key]: v }));
  }

  // Validate the mapping. Returns an error message or null when good to go.
  const validation = useMemo(() => {
    for (const k of REQUIRED_KEYS) {
      if (mapping[k] < 0) return `Map a column for ${COLUMN_LABELS[k]}.`;
    }
    if (hasDebitCredit) {
      if (mapping.debit < 0 || mapping.credit < 0) {
        return "Map both Debit and Credit columns, or switch to a single Amount column.";
      }
    } else {
      if (mapping.amount < 0) return "Map a column for Amount.";
    }
    return null;
  }, [mapping, hasDebitCredit]);

  function buildImportInput(): CsvImportInput {
    // Strip -1 sentinels — the API expects real column indexes.
    const m: Record<string, number> = {};
    for (const [k, v] of Object.entries(mapping)) {
      if (v >= 0) m[k] = v;
    }
    if (hasDebitCredit) {
      // The backend looks at `debit` + `credit` and ignores `amount`; drop
      // `amount` so the request is minimal.
      delete m.amount;
    } else {
      delete m.debit;
      delete m.credit;
    }
    return {
      file,
      columnMapping: m,
      positiveIsDebit,
      hasDebitCredit,
      dateFormat: preview.date_format,
      ...(appendToConnectionId
        ? { connectionId: appendToConnectionId }
        : {
            userId,
            accountName: accountName.trim() || "CSV Import",
          }),
    };
  }

  return (
    <div className="flex flex-1 flex-col gap-5 overflow-y-auto">
      <Alert variant="default">
        <AlertTitle className="text-sm">
          {file.name}
          <span className="text-muted-foreground ml-2 text-xs font-normal">
            {preview.total_rows} rows · {preview.headers.length} columns
            {preview.template_name ? ` · ${preview.template_name}` : ""}
          </span>
        </AlertTitle>
        <AlertDescription className="text-xs">
          {preview.template_name
            ? "Detected template — column mapping pre-filled. Review before importing."
            : "No template detected — confirm each column is mapped to the right field."}
        </AlertDescription>
      </Alert>

      <div className="space-y-3">
        <Label className="text-muted-foreground text-xs uppercase tracking-wide">
          Map columns
        </Label>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <ColumnSelect
            id="map-date"
            label={COLUMN_LABELS.date + " *"}
            headers={preview.headers}
            value={mapping.date}
            required
            onChange={(v) => setColumn("date", v)}
          />
          <ColumnSelect
            id="map-description"
            label={COLUMN_LABELS.description + " *"}
            headers={preview.headers}
            value={mapping.description}
            required
            onChange={(v) => setColumn("description", v)}
          />
          {hasDebitCredit ? (
            <>
              <ColumnSelect
                id="map-debit"
                label={COLUMN_LABELS.debit + " *"}
                headers={preview.headers}
                value={mapping.debit}
                required
                onChange={(v) => setColumn("debit", v)}
              />
              <ColumnSelect
                id="map-credit"
                label={COLUMN_LABELS.credit + " *"}
                headers={preview.headers}
                value={mapping.credit}
                required
                onChange={(v) => setColumn("credit", v)}
              />
            </>
          ) : (
            <ColumnSelect
              id="map-amount"
              label={COLUMN_LABELS.amount + " *"}
              headers={preview.headers}
              value={mapping.amount}
              required
              onChange={(v) => setColumn("amount", v)}
            />
          )}
          {OPTIONAL_KEYS.map((k) => (
            <ColumnSelect
              key={k}
              id={`map-${k}`}
              label={COLUMN_LABELS[k]}
              headers={preview.headers}
              value={mapping[k]}
              onChange={(v) => setColumn(k, v)}
            />
          ))}
        </div>
      </div>

      <fieldset className="space-y-2">
        <Label className="text-muted-foreground text-xs uppercase tracking-wide">
          Amount format
        </Label>
        <label className="text-muted-foreground flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-primary"
            checked={hasDebitCredit}
            onChange={(e) => setHasDebitCredit(e.target.checked)}
          />
          Separate Debit and Credit columns (instead of one Amount column)
        </label>
        {!hasDebitCredit && (
          <div className="ml-6 mt-2 flex flex-col gap-1.5 text-sm">
            <label className="text-muted-foreground flex items-center gap-2">
              <input
                type="radio"
                name="csv-sign"
                className="size-4 accent-primary"
                checked={positiveIsDebit}
                onChange={() => setPositiveIsDebit(true)}
              />
              Positive numbers are charges (money out)
            </label>
            <label className="text-muted-foreground flex items-center gap-2">
              <input
                type="radio"
                name="csv-sign"
                className="size-4 accent-primary"
                checked={!positiveIsDebit}
                onChange={() => setPositiveIsDebit(false)}
              />
              Negative numbers are charges (money out)
            </label>
          </div>
        )}
      </fieldset>

      {!appendToConnectionId && (
        <div className="space-y-1.5">
          <Label
            htmlFor="account-name"
            className="text-muted-foreground text-xs uppercase tracking-wide"
          >
            Account name
          </Label>
          <Input
            id="account-name"
            value={accountName}
            onChange={(e) => setAccountName(e.target.value)}
            placeholder="CSV Import"
          />
          <p className="text-muted-foreground text-xs">
            Shown in the accounts list. You can rename it later.
          </p>
        </div>
      )}

      <div className="space-y-2">
        <Label className="text-muted-foreground text-xs uppercase tracking-wide">
          Preview ({Math.min(preview.preview_rows.length, 10)} of {preview.total_rows} rows)
        </Label>
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full text-xs">
            <thead className="bg-muted/40">
              <tr>
                {preview.headers.map((h, i) => (
                  <th key={i} className="px-2 py-1.5 text-left font-medium">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {preview.preview_rows.slice(0, 10).map((row, ri) => (
                <tr key={ri} className="border-border/60 border-t">
                  {row.map((cell, ci) => (
                    <td
                      key={ci}
                      className="text-muted-foreground max-w-[16ch] truncate px-2 py-1.5"
                    >
                      {cell}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {validation && (
        <Alert variant="default">
          <AlertDescription className="text-xs">{validation}</AlertDescription>
        </Alert>
      )}

      <div className="mt-auto flex flex-wrap items-center justify-end gap-2 pt-2">
        <Button variant="ghost" onClick={onBack} disabled={importing}>
          <X className="size-4" />
          Pick a different file
        </Button>
        <Button
          onClick={() => onImport(buildImportInput())}
          disabled={!!validation || importing}
        >
          {importing ? <Loader2 className="size-4 animate-spin" /> : null}
          Import {preview.total_rows} rows
        </Button>
      </div>
    </div>
  );
}

function ColumnSelect({
  id,
  label,
  headers,
  value,
  required,
  onChange,
}: {
  id: string;
  label: string;
  headers: string[];
  value: number;
  required?: boolean;
  onChange: (v: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id} className="text-xs">
        {label}
      </Label>
      <Select
        value={value < 0 ? "none" : String(value)}
        onValueChange={onChange}
      >
        <SelectTrigger id={id}>
          <SelectValue placeholder={required ? "Pick a column…" : "— None —"} />
        </SelectTrigger>
        <SelectContent>
          {!required && <SelectItem value="none">— None —</SelectItem>}
          {headers.map((h, i) => (
            <SelectItem key={i} value={String(i)}>
              {h}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// CsvImportFormSkeleton renders an approximate skeleton for the form — used
// by the sandbox specimen so the layout shows up without needing a live file
// preview.
export function CsvImportFormSkeleton() {
  return (
    <div className="flex flex-col gap-4">
      <Skeleton className="h-32 w-full rounded-lg" />
      <Skeleton className="h-10 w-full" />
    </div>
  );
}
