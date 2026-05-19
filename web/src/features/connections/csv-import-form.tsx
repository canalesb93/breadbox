import { useEffect, useMemo, useRef, useState } from "react";
import {
  AlertCircle,
  ArrowLeft,
  FileSpreadsheet,
  Loader2,
  Upload,
  Wand2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { StatusPanel } from "@/components/status-panel";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Eyebrow } from "@/components/eyebrow";
import { FormFooter } from "@/components/form-footer";
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
const MAX_CSV_MB = Math.floor(MAX_CSV_BYTES / 1024 / 1024);

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
        `File is too large. Keep CSVs under ${MAX_CSV_MB} MB.`,
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
    <div className="flex flex-1 flex-col gap-4 overflow-y-auto overscroll-contain [-webkit-overflow-scrolling:touch]">
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
          // Dashed drop zone. Refined the rest tone so the muted backdrop
          // reads softer than `bg-accent/30` (which renders almost-white in
          // light + a noticeably blue tint in dark), and tightened the
          // drag-over treatment so the primary tint is unambiguous.
          "group bg-muted/20 hover:border-primary/50 hover:bg-muted/40 flex min-h-[200px] cursor-pointer flex-col items-center justify-center gap-3 rounded-lg border-2 border-dashed border-border/70 p-6 text-center transition",
          dragOver &&
            "border-primary/70 bg-primary/[0.06] ring-4 ring-primary/15",
          loading && "pointer-events-none opacity-70",
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
        <span
          className={cn(
            // Icon tile vocabulary borrowed from StatusPanel / EmptyState —
            // a size-11 rounded surface with a tinted background, so the
            // affordance reads as a primitive, not a stray icon.
            "bg-background text-muted-foreground border border-border/70 group-hover:border-primary/40 group-hover:bg-primary/8 group-hover:text-primary flex size-11 items-center justify-center rounded-xl transition",
            dragOver &&
              "border-primary/50 bg-primary/10 text-primary",
            loading && "border-primary/40 bg-primary/8 text-primary",
          )}
        >
          {loading ? (
            <Loader2 className="size-5 animate-spin" />
          ) : (
            <Upload className="size-5" />
          )}
        </span>
        <div className="space-y-1">
          <div className="text-foreground text-sm font-medium">
            {loading ? "Parsing CSV…" : "Drop a CSV here, or click to browse"}
          </div>
          <p className="text-muted-foreground text-xs">
            Up to {MAX_CSV_MB} MB · we'll detect columns automatically.
          </p>
        </div>
      </label>

      <div className="text-muted-foreground flex items-start gap-2.5 rounded-md border bg-muted/20 px-3 py-2.5 text-xs">
        <Wand2 className="text-muted-foreground/80 mt-0.5 size-3.5 shrink-0" />
        <span>
          Header row required. Common bank exports (Chase, Capital One, Amex,
          BofA) are auto-detected and pre-mapped on the next step.
        </span>
      </div>

      <FormFooter
        inset="sheet"
        secondary={
          <Button
            variant="outline"
            size="sm"
            onClick={onCancel}
            disabled={loading}
          >
            Cancel
          </Button>
        }
      />
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
        debit:
          m.debit === -1 && typeof inferredDebit === "number"
            ? inferredDebit
            : m.debit,
        credit:
          m.credit === -1 && typeof inferredCredit === "number"
            ? inferredCredit
            : m.credit,
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

  const fileSizeKb = Math.max(1, Math.round(file.size / 1024));

  return (
    <div className="flex flex-1 flex-col gap-5 overflow-y-auto overscroll-contain [-webkit-overflow-scrolling:touch]">
      {/* File pill — replaces the misused Alert. Filename + meta on one
          line so the eye keeps scanning down to the mapping. */}
      <div className="bg-muted/20 flex items-start gap-3 rounded-md border px-3.5 py-2.5">
        <span className="bg-amber-500/10 text-amber-600 dark:text-amber-400 flex size-9 shrink-0 items-center justify-center rounded-md">
          <FileSpreadsheet className="size-4" />
        </span>
        <div className="min-w-0 flex-1 space-y-0.5">
          <div className="flex items-baseline gap-2">
            <p className="text-foreground truncate text-sm font-medium">
              {file.name}
            </p>
            <span className="text-muted-foreground shrink-0 text-[11px] tabular-nums">
              {fileSizeKb.toLocaleString()} KB
            </span>
          </div>
          <p className="text-muted-foreground text-xs">
            <span className="tabular-nums">{preview.total_rows}</span> rows ·{" "}
            <span className="tabular-nums">{preview.headers.length}</span>{" "}
            columns
            {preview.template_name ? (
              <>
                {" · "}
                <span className="text-foreground/80 font-medium">
                  {preview.template_name}
                </span>{" "}
                detected
              </>
            ) : null}
          </p>
        </div>
      </div>

      <section className="space-y-3">
        <Eyebrow as="p">Map columns</Eyebrow>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <ColumnSelect
            id="map-date"
            label={COLUMN_LABELS.date}
            headers={preview.headers}
            value={mapping.date}
            required
            onChange={(v) => setColumn("date", v)}
          />
          <ColumnSelect
            id="map-description"
            label={COLUMN_LABELS.description}
            headers={preview.headers}
            value={mapping.description}
            required
            onChange={(v) => setColumn("description", v)}
          />
          {hasDebitCredit ? (
            <>
              <ColumnSelect
                id="map-debit"
                label={COLUMN_LABELS.debit}
                headers={preview.headers}
                value={mapping.debit}
                required
                onChange={(v) => setColumn("debit", v)}
              />
              <ColumnSelect
                id="map-credit"
                label={COLUMN_LABELS.credit}
                headers={preview.headers}
                value={mapping.credit}
                required
                onChange={(v) => setColumn("credit", v)}
              />
            </>
          ) : (
            <ColumnSelect
              id="map-amount"
              label={COLUMN_LABELS.amount}
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
      </section>

      <section className="space-y-3">
        <Eyebrow as="p">Amount format</Eyebrow>
        <label className="flex items-start gap-2.5 text-sm">
          <Checkbox
            id="csv-has-debit-credit"
            checked={hasDebitCredit}
            onCheckedChange={(v) => setHasDebitCredit(v === true)}
            className="mt-0.5"
          />
          <span className="text-foreground/90 leading-snug">
            Separate Debit and Credit columns{" "}
            <span className="text-muted-foreground">
              (instead of one Amount column)
            </span>
          </span>
        </label>
        {!hasDebitCredit && (
          <RadioGroup
            value={positiveIsDebit ? "positive" : "negative"}
            onValueChange={(v) => setPositiveIsDebit(v === "positive")}
            className="ml-6 gap-2"
          >
            <label className="flex items-center gap-2.5 text-sm leading-none">
              <RadioGroupItem id="csv-sign-positive" value="positive" />
              <span className="text-foreground/90">
                Positive numbers are charges{" "}
                <span className="text-muted-foreground">(money out)</span>
              </span>
            </label>
            <label className="flex items-center gap-2.5 text-sm leading-none">
              <RadioGroupItem id="csv-sign-negative" value="negative" />
              <span className="text-foreground/90">
                Negative numbers are charges{" "}
                <span className="text-muted-foreground">(money out)</span>
              </span>
            </label>
          </RadioGroup>
        )}
      </section>

      {!appendToConnectionId && (
        <section className="space-y-2">
          <Eyebrow as="label" htmlFor="account-name">
            Account name
          </Eyebrow>
          <Input
            id="account-name"
            value={accountName}
            onChange={(e) => setAccountName(e.target.value)}
            placeholder="CSV Import"
          />
          <p className="text-muted-foreground text-xs">
            Shown in the accounts list. You can rename it later.
          </p>
        </section>
      )}

      <section className="space-y-2">
        <div className="flex items-baseline justify-between gap-3">
          <Eyebrow as="p">Preview</Eyebrow>
          <span className="text-muted-foreground text-[11px] tabular-nums">
            {Math.min(preview.preview_rows.length, 10)} of {preview.total_rows}{" "}
            rows
          </span>
        </div>
        <div className="overflow-x-auto overscroll-contain [-webkit-overflow-scrolling:touch] rounded-md border">
          <Table className="text-xs">
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                {preview.headers.map((h, i) => (
                  <TableHead
                    key={i}
                    className="h-8 px-2.5 text-[11px] font-medium"
                  >
                    {h}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {preview.preview_rows.slice(0, 10).map((row, ri) => (
                <TableRow key={ri}>
                  {row.map((cell, ci) => (
                    <TableCell
                      key={ci}
                      className="text-muted-foreground max-w-[16ch] truncate px-2.5 py-1.5"
                    >
                      {cell}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </section>

      {validation && (
        <StatusPanel
          tone="destructive"
          icon={AlertCircle}
          heading="Fix the column mapping"
          body={validation}
        />
      )}

      <FormFooter
        inset="sheet"
        hint={
          validation ? null : (
            <span className="text-muted-foreground text-xs">
              Ready to import{" "}
              <span className="text-foreground tabular-nums font-medium">
                {preview.total_rows}
              </span>{" "}
              rows.
            </span>
          )
        }
        secondary={
          <Button
            variant="ghost"
            size="sm"
            onClick={onBack}
            disabled={importing}
          >
            <ArrowLeft className="size-3.5" />
            Different file
          </Button>
        }
        primary={
          <Button
            size="sm"
            onClick={() => onImport(buildImportInput())}
            disabled={!!validation || importing}
          >
            {importing ? <Loader2 className="size-3.5 animate-spin" /> : null}
            Import {preview.total_rows} rows
          </Button>
        }
      />
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
      <Label
        htmlFor={id}
        className="text-foreground/80 flex items-center gap-1 text-xs font-medium"
      >
        {label}
        {required && (
          // Required indicator — a small destructive-toned asterisk that
          // doesn't shout. Mirrors the affordance used on shadcn form
          // examples without coupling to `<FormItem>`.
          <span
            aria-hidden="true"
            className="text-destructive/80 text-[11px] leading-none"
          >
            *
          </span>
        )}
      </Label>
      <Select
        value={value < 0 ? "none" : String(value)}
        onValueChange={onChange}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue
            placeholder={required ? "Pick a column…" : "— None —"}
          />
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

