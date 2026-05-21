// ⌘K command palette — the first v3 JS island.
//
// Convention (see .claude/rules/app-mpa.md → "JS islands"): an island is a tiny,
// dependency-free TS module that hydrates server-rendered DOM and wires *behavior only*.
// It must never own navigation — it only triggers real `location.href` navigations, so
// the browser keeps owning history/scroll/bfcache. With JS off, nothing appears and every
// destination stays reachable via the sidebar (pure progressive enhancement).
//
// This island creates its own <dialog> on first open (so there's no server-rendered shell
// to keep in sync and zero flash before hydration). It opens on ⌘K / Ctrl+K, closes on
// Escape (native <dialog>), filters a static list of destinations, and navigates on
// Enter/click. The <dialog> gives us the focus trap + backdrop + Esc for free.

interface Command {
  label: string;
  href: string;
  group: string;
  keywords?: string;
}

// Destinations mirror the sidebar IA (layout/nav.go) plus quick "New …" actions.
// Kept in TS deliberately: this is static IA, not server state — no round-trip needed.
const COMMANDS: Command[] = [
  // Navigate
  { label: "Home", href: "/app/", group: "Go to", keywords: "dashboard overview" },
  { label: "Transactions", href: "/app/transactions", group: "Go to", keywords: "txns spending" },
  { label: "Reports", href: "/app/reports", group: "Go to", keywords: "insights charts" },
  { label: "Accounts", href: "/app/accounts", group: "Go to", keywords: "balances" },
  { label: "Connections", href: "/app/connections", group: "Go to", keywords: "banks links" },
  { label: "Providers", href: "/app/providers", group: "Go to", keywords: "plaid teller csv" },
  { label: "Categories", href: "/app/categories", group: "Go to" },
  { label: "Tags", href: "/app/tags", group: "Go to" },
  { label: "Rules", href: "/app/rules", group: "Go to", keywords: "automation" },
  { label: "Agents", href: "/app/agents", group: "Go to", keywords: "ai claude" },
  { label: "API keys", href: "/app/api-keys", group: "Go to", keywords: "tokens" },
  { label: "Settings", href: "/app/settings", group: "Go to", keywords: "preferences config" },
  // Create
  { label: "New category", href: "/app/categories?new=1", group: "Create" },
  { label: "New tag", href: "/app/tags?new=1", group: "Create" },
  { label: "New rule", href: "/app/rules?new=1", group: "Create" },
  { label: "New agent", href: "/app/agents?new=1", group: "Create" },
  { label: "New API key", href: "/app/api-keys?new=1", group: "Create" },
];

function matches(cmd: Command, q: string): boolean {
  if (!q) return true;
  const hay = (cmd.label + " " + cmd.group + " " + (cmd.keywords ?? "")).toLowerCase();
  return q
    .toLowerCase()
    .split(/\s+/)
    .every((tok) => hay.includes(tok));
}

class CommandPalette {
  private dialog: HTMLDialogElement;
  private input: HTMLInputElement;
  private list: HTMLUListElement;
  private results: Command[] = [];
  private active = 0;

  constructor() {
    this.dialog = document.createElement("dialog");
    this.dialog.className = "palette";
    this.dialog.setAttribute("aria-label", "Command palette");
    this.dialog.innerHTML = `
      <div class="palette-panel" role="combobox" aria-expanded="true" aria-haspopup="listbox">
        <div class="palette-search">
          <svg class="palette-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.3-4.3"></path></svg>
          <input class="palette-input" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Search or jump to…" aria-label="Search commands" aria-controls="palette-list" aria-autocomplete="list" />
          <kbd class="palette-esc">esc</kbd>
        </div>
        <ul class="palette-list" id="palette-list" role="listbox"></ul>
        <div class="palette-empty" hidden>No matches.</div>
      </div>`;
    document.body.appendChild(this.dialog);

    this.input = this.dialog.querySelector(".palette-input") as HTMLInputElement;
    this.list = this.dialog.querySelector(".palette-list") as HTMLUListElement;

    this.input.addEventListener("input", () => this.render());
    this.input.addEventListener("keydown", (e) => this.onKeydown(e));
    // Click outside the panel (on the backdrop) closes.
    this.dialog.addEventListener("click", (e) => {
      if (e.target === this.dialog) this.close();
    });
    this.dialog.addEventListener("close", () => {
      this.input.value = "";
    });
  }

  open(): void {
    if (this.dialog.open) return;
    this.render();
    this.dialog.showModal();
    this.input.focus();
  }

  close(): void {
    if (this.dialog.open) this.dialog.close();
  }

  toggle(): void {
    this.dialog.open ? this.close() : this.open();
  }

  private render(): void {
    const q = this.input.value.trim();
    this.results = COMMANDS.filter((c) => matches(c, q));
    this.active = 0;
    const empty = this.dialog.querySelector(".palette-empty") as HTMLElement;
    empty.hidden = this.results.length > 0;

    let group = "";
    const frag = document.createDocumentFragment();
    this.results.forEach((cmd, i) => {
      if (cmd.group !== group) {
        group = cmd.group;
        const h = document.createElement("li");
        h.className = "palette-group";
        h.setAttribute("role", "presentation");
        h.textContent = group;
        frag.appendChild(h);
      }
      const li = document.createElement("li");
      li.className = "palette-item";
      li.setAttribute("role", "option");
      li.dataset.index = String(i);
      li.textContent = cmd.label;
      li.addEventListener("click", () => this.go(i));
      li.addEventListener("mousemove", () => this.setActive(i));
      frag.appendChild(li);
    });
    this.list.replaceChildren(frag);
    this.highlight();
  }

  private setActive(i: number): void {
    this.active = i;
    this.highlight();
  }

  private highlight(): void {
    const items = this.list.querySelectorAll<HTMLElement>(".palette-item");
    items.forEach((el) => {
      const i = Number(el.dataset.index);
      const on = i === this.active;
      el.classList.toggle("is-active", on);
      el.setAttribute("aria-selected", on ? "true" : "false");
      if (on) el.scrollIntoView({ block: "nearest" });
    });
  }

  private onKeydown(e: KeyboardEvent): void {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (this.results.length) this.setActive((this.active + 1) % this.results.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (this.results.length)
        this.setActive((this.active - 1 + this.results.length) % this.results.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      this.go(this.active);
    }
    // Escape is handled natively by <dialog>.
  }

  private go(i: number): void {
    const cmd = this.results[i];
    if (!cmd) return;
    this.close();
    // The island's only navigation: a real document navigation. No client router.
    location.href = cmd.href;
  }
}

let palette: CommandPalette | null = null;

function ensure(): CommandPalette {
  if (!palette) palette = new CommandPalette();
  return palette;
}

document.addEventListener("keydown", (e) => {
  const k = e.key.toLowerCase();
  if ((e.metaKey || e.ctrlKey) && k === "k") {
    e.preventDefault();
    ensure().toggle();
  }
});

// A topbar affordance (rendered server-side with [data-palette-open]) opens it on click.
document.addEventListener("click", (e) => {
  const trigger = (e.target as Element | null)?.closest("[data-palette-open]");
  if (trigger) {
    e.preventDefault();
    ensure().open();
  }
});
