import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { getApiKey, setApiKey, clearApiKey } from "../api";

export function Layout({ children }: { children: React.ReactNode }) {
  const [key, setKey] = useState(getApiKey() ?? "");
  const [editing, setEditing] = useState(!getApiKey());

  const save = () => {
    if (!key.startsWith("bb_")) {
      alert("API keys start with 'bb_'");
      return;
    }
    setApiKey(key);
    setEditing(false);
    window.location.reload();
  };

  const reset = () => {
    clearApiKey();
    setKey("");
    setEditing(true);
  };

  return (
    <div style={styles.shell}>
      <header style={styles.header}>
        <div style={styles.brand}>
          <strong>Breadbox</strong>
          <span style={styles.tag}>web prototype</span>
        </div>
        <nav style={styles.nav}>
          <Link to="/" style={styles.navLink} activeProps={{ style: styles.navLinkActive }}>
            Home
          </Link>
          <Link to="/accounts" style={styles.navLink} activeProps={{ style: styles.navLinkActive }}>
            Accounts
          </Link>
          <Link to="/categories" style={styles.navLink} activeProps={{ style: styles.navLinkActive }}>
            Categories
          </Link>
        </nav>
        <div style={styles.auth}>
          {editing ? (
            <>
              <input
                type="password"
                placeholder="bb_..."
                value={key}
                onChange={(e) => setKey(e.target.value)}
                style={styles.input}
              />
              <button onClick={save} style={styles.button}>
                Save
              </button>
            </>
          ) : (
            <>
              <span style={styles.muted}>API key set</span>
              <button onClick={reset} style={styles.buttonGhost}>
                Reset
              </button>
            </>
          )}
        </div>
      </header>
      <main style={styles.main}>{children}</main>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  shell: {
    fontFamily:
      "ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, sans-serif",
    color: "#1c1917",
    minHeight: "100vh",
    background: "#fafaf9",
  },
  header: {
    display: "flex",
    alignItems: "center",
    gap: 16,
    padding: "12px 24px",
    borderBottom: "1px solid #e7e5e4",
    background: "white",
    position: "sticky",
    top: 0,
    zIndex: 10,
  },
  brand: { display: "flex", alignItems: "baseline", gap: 8 },
  tag: { fontSize: 12, color: "#a8a29e" },
  nav: { display: "flex", gap: 8, marginLeft: 24 },
  navLink: {
    padding: "6px 10px",
    borderRadius: 6,
    color: "#57534e",
    textDecoration: "none",
    fontSize: 14,
  },
  navLinkActive: {
    color: "#1c1917",
    background: "#f5f5f4",
    fontWeight: 600,
  },
  auth: { marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 },
  input: {
    padding: "6px 10px",
    border: "1px solid #d6d3d1",
    borderRadius: 6,
    fontSize: 13,
    width: 220,
  },
  button: {
    padding: "6px 12px",
    background: "#1c1917",
    color: "white",
    border: "none",
    borderRadius: 6,
    fontSize: 13,
    cursor: "pointer",
  },
  buttonGhost: {
    padding: "6px 12px",
    background: "transparent",
    color: "#57534e",
    border: "1px solid #d6d3d1",
    borderRadius: 6,
    fontSize: 13,
    cursor: "pointer",
  },
  muted: { fontSize: 13, color: "#78716c" },
  main: { padding: "24px" },
};
