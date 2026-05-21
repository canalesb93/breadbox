// Transactions list island — page-scoped (loaded only by the transactions list
// template, never base.templ). Two behaviors, both pure progressive enhancement:
//
//   1. Bulk select + action bar. The table is already a native multi-checkbox
//      <form data-tx-bulk> that POSTs checked tx_id[]s + category/tag to
//      /app/transactions/bulk. This island wires the select-all checkbox, tracks
//      the selected count, and shows/hides the sticky action bar. With JS off the
//      bar renders statically (the [hidden] attr is removed below only when JS runs),
//      so the bulk submit still works — the island only enhances UX.
//
//   2. Inline category edit. Each category cell has a label button + a hidden
//      <select data-tx-cat>. Clicking the label reveals the select; on change the
//      island POSTs {category_slug} to /app/transactions/{id}/category as fetch and
//      optimistically swaps the label text. With JS off the select stays hidden and
//      the label (a link target on the row) plus the detail page remain the path.
//
// The island never owns navigation. On the bulk path the browser submits the form
// (a real document POST → 303). The inline path is the one fetch here; on success it
// updates a cell in place (no nav), on failure it reloads to the server truth.

function initBulk(form: HTMLFormElement): void {
  const bar = form.querySelector<HTMLElement>("[data-tx-bulkbar]");
  const all = form.querySelector<HTMLInputElement>("[data-tx-all]");
  const count = form.querySelector<HTMLElement>("[data-tx-count]");
  const clear = form.querySelector<HTMLButtonElement>("[data-tx-clear]");
  const checks = (): HTMLInputElement[] =>
    Array.from(form.querySelectorAll<HTMLInputElement>("[data-tx-check]"));

  if (!bar) return;
  // JS is present — the bar now hides until a row is selected (it renders
  // un-hidden server-side so no-JS users always have it).
  bar.hidden = true;

  function selected(): HTMLInputElement[] {
    return checks().filter((c) => c.checked);
  }

  function sync(): void {
    const n = selected().length;
    if (count) count.textContent = `${n} selected`;
    if (bar) bar.hidden = n === 0;
    if (all) {
      const boxes = checks();
      all.checked = boxes.length > 0 && n === boxes.length;
      all.indeterminate = n > 0 && n < boxes.length;
    }
  }

  form.addEventListener("change", (e) => {
    const t = e.target as HTMLElement | null;
    if (t && t.matches("[data-tx-check]")) sync();
  });

  all?.addEventListener("change", () => {
    const on = all.checked;
    checks().forEach((c) => {
      c.checked = on;
    });
    sync();
  });

  clear?.addEventListener("click", () => {
    checks().forEach((c) => {
      c.checked = false;
    });
    sync();
  });

  sync();
}

function initInline(): void {
  document.querySelectorAll<HTMLElement>("[data-tx-catcell]").forEach((cell) => {
    const editBtn = cell.querySelector<HTMLButtonElement>("[data-tx-cat-edit]");
    const label = cell.querySelector<HTMLElement>("[data-tx-cat-label]");
    const select = cell.querySelector<HTMLSelectElement>("[data-tx-cat]");
    const id = cell.dataset.txId;
    const ret = cell.dataset.txReturn || "/app/transactions";
    if (!editBtn || !label || !select || !id) return;

    function reveal(): void {
      label!.hidden = true;
      select!.hidden = false;
      select!.classList.remove("hidden");
      select!.focus();
    }
    function conceal(): void {
      select!.hidden = true;
      select!.classList.add("hidden");
      label!.hidden = false;
    }

    editBtn.addEventListener("click", reveal);
    select.addEventListener("blur", conceal);

    select.addEventListener("change", async () => {
      const slug = select.value;
      if (!slug) {
        conceal();
        return;
      }
      const chosen = select.options[select.selectedIndex]?.text ?? slug;
      const body = new URLSearchParams({ category_slug: slug, return: ret });
      try {
        const res = await fetch(`/app/transactions/${encodeURIComponent(id!)}/category`, {
          method: "POST",
          headers: { "Content-Type": "application/x-www-form-urlencoded" },
          body: body.toString(),
          credentials: "same-origin",
          // The handler 303s back to the list; we don't want to follow that as
          // a fetch redirect, just confirm success.
          redirect: "manual",
        });
        // A same-origin 303 surfaces as an opaqueredirect (res.type) under
        // redirect:"manual"; treat that and 2xx as success.
        if (res.ok || res.type === "opaqueredirect" || res.status === 0) {
          label!.textContent = chosen;
          conceal();
        } else {
          location.reload();
        }
      } catch {
        location.reload();
      }
    });
  });
}

function init(): void {
  const form = document.querySelector<HTMLFormElement>("[data-tx-bulk]");
  if (form) initBulk(form);
  initInline();
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
