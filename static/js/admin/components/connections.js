// Connections list Alpine factories for /connections.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Two factories ship from this module:
//
//   - `syncAllBtn` — page-level "Sync All" button. Tracks state and disables
//     itself for 8s after a successful POST so accidental re-clicks don't
//     fan out duplicate jobs.
//   - `syncBtn` — per-row sync button. Reads its connection ID from
//     `data-conn-id` on the x-data root (set by the templ via
//     `<div x-data="syncBtn" data-conn-id={ c.ID }>`) so the factory body
//     keeps the no-arg shape the convention requires.
//
// `bbConnNav` is the keyboard-navigation module backing j/k/Enter/s.
// Stashed on `window` so the shortcut handlers below can reach it without
// closure gymnastics, and so it stays debuggable from the devtools console.
//
// All shortcut bindings flow through the global Alpine `shortcuts` store,
// per .claude/rules/ui.md → "Keyboard shortcuts".

// Keyboard navigation for the connections list. Reads the DOM for visible
// rows (respecting the family-member filter via data-filter-user + Alpine's
// x-show display:none) instead of mirroring connection data into a store.
// The base.html dispatcher already guards against input focus, overlays,
// and touch devices, so these handlers just do the thing.
window.bbConnNav = {
  focusedIdx: -1,
  FOCUSED_CLASS: 'bb-tx-row--focused', // reuse the tx-list focus styling
  visibleRows: function () {
    var all = document.querySelectorAll('[data-connection-row]');
    var out = [];
    for (var i = 0; i < all.length; i++) {
      // offsetParent is null when x-show has display:none'd the row
      if (all[i].offsetParent !== null) out.push(all[i]);
    }
    return out;
  },
  clearFocus: function () {
    var rows = document.querySelectorAll('[data-connection-row].' + this.FOCUSED_CLASS);
    for (var i = 0; i < rows.length; i++) rows[i].classList.remove(this.FOCUSED_CLASS);
  },
  setFocus: function (idx) {
    var rows = this.visibleRows();
    if (!rows.length) { this.focusedIdx = -1; return; }
    if (idx < 0) idx = 0;
    if (idx >= rows.length) idx = rows.length - 1;
    this.clearFocus();
    rows[idx].classList.add(this.FOCUSED_CLASS);
    this.focusedIdx = idx;
    rows[idx].scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  },
  next: function () {
    var rows = this.visibleRows();
    if (!rows.length) return;
    this.setFocus(this.focusedIdx < 0 ? 0 : this.focusedIdx + 1);
  },
  prev: function () {
    var rows = this.visibleRows();
    if (!rows.length) return;
    this.setFocus(this.focusedIdx < 0 ? 0 : this.focusedIdx - 1);
  },
  currentRow: function () {
    var rows = this.visibleRows();
    if (this.focusedIdx < 0 || this.focusedIdx >= rows.length) return null;
    return rows[this.focusedIdx];
  },
};

document.addEventListener('alpine:init', function () {
  // Page-level "Sync All" button. Triggers a sync across every connection
  // owned by the current user. Disables itself for 8s after a successful
  // POST so accidental re-clicks don't fan out duplicate jobs.
  Alpine.data('syncAllBtn', function () {
    return {
      state: 'idle',

      init: function () {
        // No initial state to parse — the factory is purely behavior.
      },

      triggerSyncAll: function () {
        if (this.state !== 'idle') return;
        this.state = 'syncing';
        var self = this;
        fetch('/-/connections/sync-all', { method: 'POST' })
          .then(function (res) {
            if (res.ok) {
              self.state = 'done';
              self.$nextTick(function () { lucide.createIcons(); });
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Sync triggered for all connections.', type: 'success' } }));
              setTimeout(function () { self.state = 'idle'; self.$nextTick(function () { lucide.createIcons(); }); }, 8000);
            } else {
              return res.json().then(function (data) {
                window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: data.error || 'Failed to trigger sync.', type: 'error' } }));
                self.state = 'idle';
                self.$nextTick(function () { lucide.createIcons(); });
              });
            }
          })
          .catch(function () {
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Network error. Please try again.', type: 'error' } }));
            self.state = 'idle';
            self.$nextTick(function () { lucide.createIcons(); });
          });
      },
    };
  });

  // Per-row sync button. The wrapping element passes the connection ID via
  // `data-conn-id`; the factory reads it once in init() so the body keeps
  // the no-arg shape the convention requires.
  Alpine.data('syncBtn', function () {
    return {
      state: 'idle',
      connId: '',

      init: function () {
        this.connId = this.$el.dataset.connId || '';
      },

      triggerSync: function () {
        if (this.state !== 'idle') return;
        if (!this.connId) return;
        this.state = 'syncing';
        var self = this;
        fetch('/-/connections/' + this.connId + '/sync', { method: 'POST' })
          .then(function (res) {
            if (res.ok) {
              self.state = 'done';
              self.$nextTick(function () { lucide.createIcons(); });
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Sync triggered for this connection.', type: 'success' } }));
              setTimeout(function () { self.state = 'idle'; self.$nextTick(function () { lucide.createIcons(); }); }, 5000);
            } else {
              return res.json().then(function (data) {
                window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: data.error || 'Failed to trigger sync.', type: 'error' } }));
                self.state = 'idle';
                self.$nextTick(function () { lucide.createIcons(); });
              });
            }
          })
          .catch(function () {
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Network error. Please try again.', type: 'error' } }));
            self.state = 'idle';
            self.$nextTick(function () { lucide.createIcons(); });
          });
      },
    };
  });

  var reg = Alpine.store('shortcuts');
  if (!reg) return;

  reg.register({
    id: 'connections.next',
    keys: 'j',
    description: 'Move down',
    group: 'Navigation',
    scope: 'connections',
    action: function () { window.bbConnNav.next(); },
  });

  reg.register({
    id: 'connections.prev',
    keys: 'k',
    description: 'Move up',
    group: 'Navigation',
    scope: 'connections',
    action: function () { window.bbConnNav.prev(); },
  });

  reg.register({
    id: 'connections.open',
    keys: 'Enter',
    description: 'Open connection',
    group: 'Actions',
    scope: 'connections',
    action: function () {
      var row = window.bbConnNav.currentRow();
      if (!row) return;
      var url = row.getAttribute('data-open-url');
      if (url) window.location.href = url;
    },
  });

  reg.register({
    id: 'connections.sync',
    keys: 's',
    description: 'Sync connection',
    group: 'Actions',
    scope: 'connections',
    action: function () {
      var row = window.bbConnNav.currentRow();
      if (!row) return;
      // Reuse the existing syncBtn Alpine component's click handler so CSRF,
      // toast, and UI-state logic (disabled, spinner, 5s cooldown) stay in
      // one place. Inactive and CSV connections don't render the button —
      // no-op in that case.
      var btn = row.querySelector('[data-sync-btn]');
      if (btn) btn.click();
    },
  });
});
