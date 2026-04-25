package pages

// rulesScripts is the static JS body for the rules list page. Lifted
// verbatim from the old rules.html so the Alpine factory + keyboard
// navigation bbRuleNav module continue to work without churn. Inlined
// via @templ.Raw because templ's `{ }` interpolation would otherwise
// fight the JS object literals.
const rulesScripts = `<script>
function bbSetRulesPerPage(val) {
  const u = new URL(window.location.href);
  u.searchParams.set('per_page', val);
  u.searchParams.delete('page');
  window.location.href = u.toString();
}
function bbSetRulesSort(val) {
  const u = new URL(window.location.href);
  if (val) u.searchParams.set('sort_by', val);
  else u.searchParams.delete('sort_by');
  u.searchParams.delete('page');
  window.location.href = u.toString();
}
function rulesPage() {
  return {
    async toggleRule(id) {
      try {
        const resp = await fetch('/-/rules/' + id + '/toggle', { method: 'POST' });
        if (resp.ok) location.reload();
      } catch (e) {
        // ignore
      }
    },
    async deleteRule(id, el) {
      const ok = await bbConfirm({title:'Delete this rule?',message:'This rule will be permanently deleted. This cannot be undone.',confirmLabel:'Delete',variant:'danger'});
      if (!ok) return;
      try {
        const resp = await fetch('/-/rules/' + id, { method: 'DELETE' });
        if (resp.ok) location.reload();
      } catch (e) {
        // ignore
      }
    }
  };
}

// Keyboard navigation for the rules list. Reads the DOM for visible rows
// (no filter exists today, but offsetParent !== null stays future-proof)
// instead of mirroring rule data into a store. The base.html dispatcher
// already guards against input focus, overlays, and touch devices.
var bbRuleNav = {
  focusedIdx: -1,
  FOCUSED_CLASS: 'bb-tx-row--focused', // reuse the tx-list focus styling
  visibleRows: function() {
    var all = document.querySelectorAll('[data-rule-row]');
    var out = [];
    for (var i = 0; i < all.length; i++) {
      if (all[i].offsetParent !== null) out.push(all[i]);
    }
    return out;
  },
  clearFocus: function() {
    var rows = document.querySelectorAll('[data-rule-row].' + this.FOCUSED_CLASS);
    for (var i = 0; i < rows.length; i++) rows[i].classList.remove(this.FOCUSED_CLASS);
  },
  setFocus: function(idx) {
    var rows = this.visibleRows();
    if (!rows.length) { this.focusedIdx = -1; return; }
    if (idx < 0) idx = 0;
    if (idx >= rows.length) idx = rows.length - 1;
    this.clearFocus();
    rows[idx].classList.add(this.FOCUSED_CLASS);
    this.focusedIdx = idx;
    rows[idx].scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  },
  next: function() {
    var rows = this.visibleRows();
    if (!rows.length) return;
    this.setFocus(this.focusedIdx < 0 ? 0 : this.focusedIdx + 1);
  },
  prev: function() {
    var rows = this.visibleRows();
    if (!rows.length) return;
    this.setFocus(this.focusedIdx < 0 ? 0 : this.focusedIdx - 1);
  },
  currentRow: function() {
    var rows = this.visibleRows();
    if (this.focusedIdx < 0 || this.focusedIdx >= rows.length) return null;
    return rows[this.focusedIdx];
  },
};

document.addEventListener('alpine:init', function() {
  var reg = Alpine.store('shortcuts');
  if (!reg) return;

  reg.register({
    id: 'rules.next',
    keys: 'j',
    description: 'Move down',
    group: 'Navigation',
    scope: 'rules',
    action: function() { bbRuleNav.next(); },
  });

  reg.register({
    id: 'rules.prev',
    keys: 'k',
    description: 'Move up',
    group: 'Navigation',
    scope: 'rules',
    action: function() { bbRuleNav.prev(); },
  });

  reg.register({
    id: 'rules.edit',
    keys: 'Enter',
    description: 'Edit rule',
    group: 'Actions',
    scope: 'rules',
    action: function() {
      var row = bbRuleNav.currentRow();
      if (!row) return;
      // Prefer the dedicated edit URL; fall back to the detail page (same
      // destination as clicking the row).
      var url = row.getAttribute('data-edit-url') || row.getAttribute('data-open-url');
      if (url) window.location.href = url;
    },
  });

  reg.register({
    id: 'rules.new',
    keys: 'n',
    description: 'New rule',
    group: 'Actions',
    scope: 'rules',
    // This shadows the global ` + "`n+_`" + ` chord while on /rules — see the
    // hasChordStartingWith guard in base.html. Users can still use ` + "`g r`" + `
    // etc. to navigate; ` + "`n+r`" + ` chord lives everywhere else.
    action: function() {
      var btn = document.querySelector('[data-new-rule]');
      if (btn) btn.click();
    },
  });

  reg.register({
    id: 'rules.toggle',
    keys: ' ', // Space key — e.key is a literal space
    description: 'Toggle enabled',
    group: 'Actions',
    scope: 'rules',
    // Only fire when a row is focused, otherwise let the browser scroll.
    // The dispatcher 'when' predicate gates the match itself; if Space
    // has no focused row, no shortcut matches and default scroll runs.
    when: function() { return bbRuleNav.focusedIdx >= 0; },
    action: function() {
      var row = bbRuleNav.currentRow();
      if (!row) return;
      var btn = row.querySelector('[data-toggle-enabled]');
      if (btn) btn.click();
    },
  });

  reg.register({
    id: 'rules.delete',
    keys: 'd',
    description: 'Delete rule',
    group: 'Actions',
    scope: 'rules',
    // System rules render a disabled shield button instead of the delete
    // button — data-delete-action is absent there, so 'd' is a no-op on
    // system rules. The existing deleteRule flow shows bbConfirm() before
    // issuing the DELETE, so the confirm dialog still fires.
    action: function() {
      var row = bbRuleNav.currentRow();
      if (!row) return;
      var btn = row.querySelector('[data-delete-action]');
      if (btn) btn.click();
    },
  });
});
</script>`
