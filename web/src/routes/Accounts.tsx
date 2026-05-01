import { useQuery } from "@tanstack/react-query";
import { api, ApiError, type Account } from "../api";

function formatBalance(amount?: string | null, currency?: string | null) {
  if (!amount) return "—";
  const n = Number(amount);
  if (!Number.isFinite(n)) return amount;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: currency ?? "USD",
  }).format(n);
}

export function AccountsPage() {
  const { data, isLoading, error, refetch, isRefetching } = useQuery({
    queryKey: ["accounts"],
    queryFn: () => api<Account[]>("/api/v1/accounts"),
  });

  if (isLoading) return <p>Loading accounts…</p>;

  if (error) {
    const apiErr = error instanceof ApiError ? error : null;
    return (
      <div style={{ color: "#b91c1c" }}>
        <strong>Failed to load accounts</strong>
        <div style={{ marginTop: 4, fontSize: 13 }}>
          {apiErr ? `${apiErr.code}: ${apiErr.message}` : (error as Error).message}
        </div>
      </div>
    );
  }

  const accounts = data ?? [];

  return (
    <div>
      <header style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 16 }}>
        <h1 style={{ fontSize: 24, margin: 0 }}>Accounts</h1>
        <span style={{ color: "#a8a29e", fontSize: 14 }}>{accounts.length} total</span>
        <button
          onClick={() => refetch()}
          disabled={isRefetching}
          style={{
            marginLeft: "auto",
            padding: "6px 12px",
            border: "1px solid #d6d3d1",
            borderRadius: 6,
            background: "white",
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          {isRefetching ? "Refreshing…" : "Refresh"}
        </button>
      </header>

      <div style={{ background: "white", border: "1px solid #e7e5e4", borderRadius: 8, overflow: "hidden" }}>
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 14 }}>
          <thead>
            <tr style={{ background: "#f5f5f4", textAlign: "left" }}>
              <th style={th}>Name</th>
              <th style={th}>Type</th>
              <th style={th}>Mask</th>
              <th style={{ ...th, textAlign: "right" }}>Current</th>
              <th style={{ ...th, textAlign: "right" }}>Available</th>
            </tr>
          </thead>
          <tbody>
            {accounts.map((a) => (
              <tr key={a.id} style={{ borderTop: "1px solid #f5f5f4" }}>
                <td style={td}>
                  <div style={{ fontWeight: 500 }}>{a.name}</div>
                  {a.official_name && (
                    <div style={{ fontSize: 12, color: "#a8a29e" }}>{a.official_name}</div>
                  )}
                </td>
                <td style={td}>
                  {a.type}
                  {a.subtype ? <span style={{ color: "#a8a29e" }}> · {a.subtype}</span> : null}
                </td>
                <td style={td}>{a.mask ?? "—"}</td>
                <td style={{ ...td, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>
                  {formatBalance(a.current_balance, a.iso_currency_code)}
                </td>
                <td style={{ ...td, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>
                  {formatBalance(a.available_balance, a.iso_currency_code)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: "10px 14px", fontSize: 12, color: "#57534e", fontWeight: 600 };
const td: React.CSSProperties = { padding: "10px 14px" };
