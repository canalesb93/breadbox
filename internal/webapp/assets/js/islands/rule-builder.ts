// Rule-builder island — page-scoped (loaded only by the RuleForm template, never
// base.templ). Pure progressive enhancement over the existing native rule form.
//
// The server renders a flat list of condition rows (condition_field[]/condition_op[]/
// condition_value[]) and action rows (action_type[]/action_value[]) plus a hidden
// <template> blank row for each group. The handler (parseRuleForm) zips these []-named
// inputs positionally and skips blank rows — so the only contract this island must keep
// is: every input it adds keeps the SAME []-name. It never touches the handler.
//
// Behaviors, all additive:
//   1. "Add condition" / "Add action" — clone the group's <template> blank row and
//      append it. This removes the fixed server-side row cap.
//   2. Per-row "×" remove button — drops a row (kept ≥1 visible row per group so the
//      form never collapses to nothing; the handler tolerates a single blank row).
//   3. Action rows are draggable (native HTML5 DnD, no library) so the user can reorder
//      them — action order can matter. Conditions are ANDed leaves, so their order is
//      cosmetic; condition rows are not draggable.
//
// With JS off: the server-rendered rows submit exactly as today, the Add buttons and
// drag handles are hidden ([data-rule-jsonly] is revealed only here), and the <template>
// is inert. Nothing breaks — it's just the fixed row count.
//
// Nested AND/OR/NOT condition groups remain deferred (Phase 4): this island only manages
// the flat leaf list the form already speaks.

function initGroup(group: HTMLElement): void {
  const tpl = group.querySelector<HTMLTemplateElement>("template[data-rule-rowtpl]");
  const addBtn = group.querySelector<HTMLButtonElement>("[data-rule-add]");
  const list = group.querySelector<HTMLElement>("[data-rule-rows]");
  if (!tpl || !addBtn || !list) return;

  const draggable = group.dataset.ruleGroup === "action";

  function rows(): HTMLElement[] {
    return Array.from(list!.querySelectorAll<HTMLElement>("[data-rule-row]"));
  }

  // A row must always have a remove button (server-rendered rows include one too); the
  // handler is happy with a lone blank row, but we never let the list reach zero rows so
  // there's always a visible affordance to fill in.
  function wireRow(row: HTMLElement): void {
    const remove = row.querySelector<HTMLButtonElement>("[data-rule-remove]");
    remove?.addEventListener("click", () => {
      if (rows().length <= 1) {
        // Last row — clear its inputs instead of removing, so the group stays usable.
        row.querySelectorAll<HTMLInputElement | HTMLSelectElement>("input,select").forEach((el) => {
          if (el instanceof HTMLSelectElement) el.selectedIndex = 0;
          else el.value = "";
        });
        return;
      }
      row.remove();
    });

    if (draggable) wireDrag(row);
  }

  function wireDrag(row: HTMLElement): void {
    const handle = row.querySelector<HTMLElement>("[data-rule-drag]");
    if (!handle) return;
    handle.addEventListener("mousedown", () => {
      row.setAttribute("draggable", "true");
    });
    row.addEventListener("dragend", () => {
      row.removeAttribute("draggable");
      row.classList.remove("rule-row-dragging");
    });
    row.addEventListener("dragstart", (e) => {
      row.classList.add("rule-row-dragging");
      e.dataTransfer?.setData("text/plain", "");
      if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
    });
  }

  if (draggable) {
    // Reorder on dragover: move the dragged row before/after the row under the cursor.
    list.addEventListener("dragover", (e) => {
      const dragging = list!.querySelector<HTMLElement>(".rule-row-dragging");
      if (!dragging) return;
      e.preventDefault();
      const after = rowAfter(list!, e.clientY);
      if (after == null) list!.appendChild(dragging);
      else if (after !== dragging) list!.insertBefore(dragging, after);
    });
  }

  addBtn.addEventListener("click", () => {
    const frag = tpl!.content.cloneNode(true) as DocumentFragment;
    const row = frag.querySelector<HTMLElement>("[data-rule-row]");
    if (!row) return;
    list!.appendChild(frag);
    wireRow(row);
    row.querySelector<HTMLInputElement | HTMLSelectElement>("input,select")?.focus();
  });

  rows().forEach(wireRow);
}

// rowAfter returns the first row whose vertical midpoint is below the cursor, or null to
// drop at the end. Standard native-DnD reorder math, excludes the dragged row itself.
function rowAfter(list: HTMLElement, y: number): HTMLElement | null {
  const candidates = Array.from(
    list.querySelectorAll<HTMLElement>("[data-rule-row]:not(.rule-row-dragging)"),
  );
  let closest: { offset: number; el: HTMLElement } | null = null;
  for (const el of candidates) {
    const box = el.getBoundingClientRect();
    const offset = y - box.top - box.height / 2;
    if (offset < 0 && (closest === null || offset > closest.offset)) {
      closest = { offset, el };
    }
  }
  return closest?.el ?? null;
}

function init(): void {
  document.querySelectorAll<HTMLElement>("[data-rule-group]").forEach(initGroup);
  // Reveal the JS-only affordances (Add buttons, drag handles) now that we've hydrated.
  document.querySelectorAll<HTMLElement>("[data-rule-jsonly]").forEach((el) => {
    el.hidden = false;
  });
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
