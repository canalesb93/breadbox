import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, ApiError, type Category } from "../api";

export function CategoriesPage() {
  const [q, setQ] = useState("");
  const { data, isLoading, error } = useQuery({
    queryKey: ["categories"],
    queryFn: () => api<Category[]>("/api/v1/categories"),
  });

  const filtered = useMemo(() => {
    const list = data ?? [];
    if (!q.trim()) return list;
    const needle = q.toLowerCase();
    return list.filter((c) => c.name.toLowerCase().includes(needle));
  }, [data, q]);

  if (isLoading) return <p>Loading categories…</p>;

  if (error) {
    const apiErr = error instanceof ApiError ? error : null;
    return (
      <div style={{ color: "#b91c1c" }}>
        <strong>Failed to load categories</strong>
        <div style={{ marginTop: 4, fontSize: 13 }}>
          {apiErr ? `${apiErr.code}: ${apiErr.message}` : (error as Error).message}
        </div>
      </div>
    );
  }

  return (
    <div>
      <header style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 16 }}>
        <h1 style={{ fontSize: 24, margin: 0 }}>Categories</h1>
        <span style={{ color: "#a8a29e", fontSize: 14 }}>
          {filtered.length} of {data?.length ?? 0}
        </span>
        <input
          placeholder="Filter…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          style={{
            marginLeft: "auto",
            padding: "6px 10px",
            border: "1px solid #d6d3d1",
            borderRadius: 6,
            fontSize: 13,
            width: 220,
          }}
        />
      </header>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
          gap: 8,
        }}
      >
        {filtered.map((c) => (
          <div
            key={c.id}
            style={{
              padding: 12,
              border: "1px solid #e7e5e4",
              borderRadius: 8,
              background: "white",
              display: "flex",
              alignItems: "center",
              gap: 10,
              opacity: c.is_archived ? 0.5 : 1,
            }}
          >
            <span
              style={{
                width: 12,
                height: 12,
                borderRadius: 999,
                background: c.color ?? "#d6d3d1",
                flexShrink: 0,
              }}
            />
            <div style={{ minWidth: 0 }}>
              <div style={{ fontWeight: 500, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                {c.name}
              </div>
              <div style={{ fontSize: 11, color: "#a8a29e" }}>
                {c.short_id}
                {c.is_system && <span style={{ marginLeft: 6 }}>· system</span>}
                {c.is_archived && <span style={{ marginLeft: 6 }}>· archived</span>}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
