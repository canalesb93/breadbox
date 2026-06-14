// Connections list Alpine factories for /connections.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Two factories ship from this module:
//
//   - `connectionsList` — the page root. Owns the Connections/Links tab
//     state (hydrated from `data-tab`) and the per-row Sync now / Disconnect
//     actions dispatched from each row's OverflowMenu (and the `s` keyboard
//     shortcut). Listening on `.window` so a `$dispatch` from inside a
//     popover menu still reaches it after it bubbles to the document.
//   - `syncAllBtn` — page-level "Sync All" button. Tracks state and disables
//     itself for 8s after a successful POST so accidental re-clicks don't
//     fan out duplicate jobs.
//
// `bbConnNav` is the keyboard-navigation module backing j/k/Enter/s.
// Stashed on `window` so the shortcut handlers below can reach it without
// closure gymnastics, and so it stays debuggable from the devtools console.
//
// All shortcut bindings flow through the global Alpine `shortcuts` store,
// per .claude/rules/ui.md → "Keyboard shortcuts".

function bbToast(message, type) {
  window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type } }));
}

// Keyboard navigation for the connections list. Reads the DOM for visible
// rows instead of mirroring connection data into a store. The base.html
// dispatcher already guards against input focus, overlays, and touch
// devices, so these handlers just do the thing.
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
  // Page root: tab state + per-row Sync now / Disconnect actions.
  Alpine.data('connectionsList', function () {
    return {
      tab: 'connections',
      // Track in-flight syncs so a double-dispatch (menu + `s`) doesn't fan
      // out duplicate jobs for the same connection.
      syncing: {},

      init: function () {
        this.tab = this.$el.dataset.tab || 'connections';
      },

      syncOne: function (id, name) {
        if (!id || this.syncing[id]) return;
        this.syncing[id] = true;
        var self = this;
        fetch('/-/connections/' + id + '/sync', { method: 'POST' })
          .then(function (res) {
            if (res.ok) {
              bbToast('Sync triggered for ' + (name || 'this connection') + '.', 'success');
            } else {
              return res.json().then(function (data) {
                bbToast(data.error || 'Failed to trigger sync.', 'error');
              });
            }
          })
          .catch(function () { bbToast('Network error. Please try again.', 'error'); })
          .finally(function () {
            setTimeout(function () { delete self.syncing[id]; }, 4000);
          });
      },

      disconnectOne: function (id, name) {
        if (!id) return;
        var self = this;
        bbConfirm({
          title: 'Disconnect ' + (name || 'this bank') + '?',
          message: 'Syncing stops and the connection is marked disconnected. Existing accounts and transactions are preserved.',
          confirmLabel: 'Disconnect',
          variant: 'danger',
        }).then(function (ok) {
          if (!ok) return;
          fetch('/-/connections/' + id, { method: 'DELETE' })
            .then(function (res) {
              if (res.ok) {
                bbToast((name || 'Connection') + ' disconnected.', 'success');
                setTimeout(function () { window.location.reload(); }, 400);
              } else {
                return res.json().then(function (data) {
                  bbToast(data.error || 'Failed to disconnect.', 'error');
                });
              }
            })
            .catch(function () { bbToast('Network error. Please try again.', 'error'); });
        });
      },
    };
  });

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
              bbToast('Sync triggered for all connections.', 'success');
              setTimeout(function () { self.state = 'idle'; self.$nextTick(function () { lucide.createIcons(); }); }, 8000);
            } else {
              return res.json().then(function (data) {
                bbToast(data.error || 'Failed to trigger sync.', 'error');
                self.state = 'idle';
                self.$nextTick(function () { lucide.createIcons(); });
              });
            }
          })
          .catch(function () {
            bbToast('Network error. Please try again.', 'error');
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
      // Only syncable (active, non-CSV) rows carry data-can-sync. Dispatch
      // the same event the OverflowMenu uses so CSRF/toast/dedup logic stays
      // in one place (the connectionsList factory, listening on .window).
      if (!row.getAttribute('data-can-sync')) return;
      var id = row.getAttribute('data-conn-id');
      if (!id) return;
      window.dispatchEvent(new CustomEvent('connection-sync', { detail: { id: id, name: '' } }));
    },
  });
});
