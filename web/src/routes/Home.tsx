export function HomePage() {
  return (
    <div style={{ maxWidth: 720 }}>
      <h1 style={{ fontSize: 28, marginBottom: 12 }}>Web prototype</h1>
      <p style={{ color: "#57534e", lineHeight: 1.6 }}>
        Vite + React + TypeScript + TanStack (Query + Router) hitting the existing Go
        backend at <code>/api/v1/*</code>. The dev server proxies to the Go server on
        port <code>8081</code>.
      </p>
      <p style={{ color: "#57534e", lineHeight: 1.6, marginTop: 12 }}>
        Set an API key in the header to load real data. Try the{" "}
        <strong>Accounts</strong> and <strong>Categories</strong> tabs — both fetch
        live from the backend through TanStack Query.
      </p>
      <ul style={{ color: "#57534e", lineHeight: 1.8, marginTop: 16, paddingLeft: 20 }}>
        <li><code>/api/v1/accounts</code> — bare JSON array (bounded resource)</li>
        <li><code>/api/v1/categories</code> — bare JSON array (bounded resource)</li>
        <li>Auth via <code>X-API-Key: bb_...</code> header</li>
      </ul>
    </div>
  );
}
