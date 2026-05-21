// agent-run-stream — v3's first SSE streaming island.
//
// Convention (see .claude/rules/app-mpa.md → "JS islands" + "Datastar / SSE"):
// an island hydrates server-rendered DOM and wires behavior only. It NEVER owns
// navigation. With JS off this island simply doesn't run and the page keeps the
// server-rendered transcript + a Refresh link (pure progressive enhancement).
//
// What it does: if the run is in_progress, the server marks the <pre id="transcript">
// with data-stream-live="true" + data-stream-url. We open an EventSource to that URL
// and append each `event: line` frame as a new transcript node. On `event: done`
// we flip the "Live" pill to the run's final status and close the stream. If the
// connection errors and the run is still live, EventSource auto-reconnects; we
// pass ?from=<count> so the server never replays lines we've already rendered.

function init(): void {
  const pre = document.getElementById("transcript");
  if (!pre || pre.getAttribute("data-stream-live") !== "true") return;
  if (typeof EventSource === "undefined") return; // no SSE → static render stands

  const base = pre.getAttribute("data-stream-url");
  if (!base) return;

  const statusEl = document.querySelector<HTMLElement>("[data-stream-status]");
  // Count server-rendered lines so we resume past them across (re)connects.
  let count = pre.querySelectorAll(".transcript-line").length;
  let source: EventSource | null = null;
  let closed = false;

  const setStatus = (state: string, label: string): void => {
    if (!statusEl) return;
    statusEl.setAttribute("data-stream-status", state);
    const text = statusEl.lastChild;
    if (text && text.nodeType === Node.TEXT_NODE) text.textContent = " " + label;
  };

  const append = (line: string): void => {
    const div = document.createElement("div");
    div.className = "transcript-line border-b border-border/30 py-0.5 last:border-0";
    div.textContent = line;
    pre.appendChild(div);
    count++;
    // Keep the newest line in view (only when already near the bottom would be
    // nicer, but a transcript viewer scrolling to follow output is expected).
    pre.scrollTop = pre.scrollHeight;
  };

  const connect = (): void => {
    if (closed) return;
    source = new EventSource(base + "?from=" + count);

    source.addEventListener("line", (e) => append((e as MessageEvent).data));

    source.addEventListener("done", (e) => {
      closed = true;
      source?.close();
      const finalStatus = (e as MessageEvent).data || "done";
      setStatus(finalStatus, finalStatus.charAt(0).toUpperCase() + finalStatus.slice(1));
      const dot = statusEl?.querySelector("span");
      dot?.classList.remove("animate-pulse", "bg-primary");
    });

    // EventSource reconnects on its own after a transient error; we only act if
    // the browser gives up entirely (readyState CLOSED).
    source.onerror = () => {
      if (source && source.readyState === EventSource.CLOSED && !closed) {
        setStatus("disconnected", "Disconnected");
      }
    };
  };

  setStatus("connecting", "Live");
  connect();

  // Don't leak the connection across bfcache restores; reconnect on return.
  window.addEventListener("pagehide", () => {
    closed = true;
    source?.close();
  });
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
