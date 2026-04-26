// Connection detail Alpine component for /connections/{id}.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// State + per-connection action URLs come in via data-* attributes on the
// x-data root (data-conn-id is the only piece needed; the per-action URLs
// are all derived from it). The page exposes a single factory that owns:
//
// - the manual sync trigger + 5-minute poll loop with optimistic UI
//   (pending row in #sync-history-list, button busy state)
// - per-connection actions: pause/resume, delete, sync-interval override
// - per-account actions: rename (display name), include/exclude toggle
// - the header "remove" confirm-then-confirm UX (formerly inline x-data)
//
// Markup-side method names match the previous global-function names
// (syncConnection, togglePause, removeConnection, updateSyncInterval,
// updateDisplayName, toggleExcluded) so the inline handlers in the templ
// component continue to read naturally as `@click="syncConnection"`.
document.addEventListener('alpine:init', function () {
  Alpine.data('connectionDetail', function () {
    var SYNC_POLL_INTERVAL_MS = 1500;
    var SYNC_POLL_MAX_MS = 5 * 60 * 1000;

    return {
      // Header "remove" confirm-then-confirm state (was an inline
      // x-data="{ confirming: false }" before extraction).
      confirming: false,

      // Internal sync-poll state. Plain non-reactive fields are fine —
      // Alpine only tracks reads from the proxy, and these are written
      // from setTimeout/fetch callbacks.
      _connID: '',
      _syncPollTimer: null,
      _syncPollStartedAt: 0,

      init: function () {
        this._connID = this.$el.dataset.connId || '';
        // If the most recent sync log row is already in_progress when the
        // page renders, kick off polling immediately (e.g. user landed
        // here while a manual/cron sync was running).
        var first = document.querySelector('#sync-history-list [data-sync-log-row]');
        if (first && first.getAttribute('data-sync-log-status') === 'in_progress') {
          this.setSyncButtonBusy();
          this.pollSyncStatus();
        }
      },

      // --- Toast helper. Mirrors the global-scope showToast in the
      // pre-extraction version so error paths read identically. ---
      showToast: function (message, type) {
        window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
      },

      // --- Sync button state ---
      setSyncButtonIdle: function () {
        var btn = document.getElementById('sync-btn');
        if (!btn) return;
        btn.disabled = false;
        btn.innerHTML = '<i data-lucide="refresh-cw" class="w-3.5 h-3.5"></i> Sync Now';
        lucide.createIcons({ nodes: [btn] });
      },

      setSyncButtonBusy: function () {
        var btn = document.getElementById('sync-btn');
        if (!btn) return;
        btn.disabled = true;
        btn.innerHTML = '<span class="loading loading-spinner loading-xs"></span> Syncing...';
      },

      // --- DOM helpers for the optimistic pending sync-log row. ---
      buildSyncLogRow: function (payload) {
        var row = document.createElement('div');
        row.className = 'flex gap-3 py-3 border-b border-base-300/30 last:border-0';
        row.setAttribute('data-sync-log-row', payload.short_id || 'pending');
        row.setAttribute('data-sync-log-status', payload.status || 'in_progress');
        row.innerHTML = ''
          + '<div class="flex flex-col items-center shrink-0 pt-0.5" data-sync-log-icon>'
          +   '<div class="w-6 h-6 rounded-full flex items-center justify-center bg-base-200">'
          +     '<span class="loading loading-spinner loading-xs text-primary"></span>'
          +   '</div>'
          + '</div>'
          + '<div class="flex-1 min-w-0 flex items-center justify-between gap-2">'
          +   '<div class="min-w-0">'
          +     '<div class="flex items-center gap-2 flex-wrap">'
          +       '<span class="text-sm font-medium capitalize"></span>'
          +       '<span class="text-xs text-base-content/35 tabular-nums" data-sync-log-time>just now</span>'
          +       '<span class="text-[0.6rem] text-base-content/25 tabular-nums bg-base-200/50 px-1.5 py-0.5 rounded-full hidden" data-sync-log-duration></span>'
          +     '</div>'
          +     '<p class="text-xs text-error/60 mt-0.5 truncate hidden" data-sync-log-error></p>'
          +   '</div>'
          +   '<div class="flex items-center gap-2.5 tabular-nums text-xs shrink-0" data-sync-log-counts>'
          +     '<div class="flex items-center gap-1.5 hidden" data-sync-log-counts-main>'
          +       '<span class="text-success font-medium hidden" data-sync-log-added></span>'
          +       '<span class="text-info font-medium hidden" data-sync-log-modified></span>'
          +       '<span class="text-error font-medium hidden" data-sync-log-removed></span>'
          +     '</div>'
          +     '<span class="text-base-content/25 text-[0.6rem] hidden" data-sync-log-unchanged></span>'
          +   '</div>'
          + '</div>';
        row.querySelector('span.capitalize').textContent = payload.trigger || 'manual';
        return row;
      },

      _toggleHidden: function (el, hidden) {
        if (!el) return;
        if (hidden) el.classList.add('hidden'); else el.classList.remove('hidden');
      },

      updateSyncLogRow: function (row, payload) {
        if (!row) return;
        var status = payload.status || 'in_progress';
        row.setAttribute('data-sync-log-status', status);
        if (payload.short_id) row.setAttribute('data-sync-log-row', payload.short_id);

        var iconWrap = row.querySelector('[data-sync-log-icon] > div');
        if (iconWrap) {
          var iconHTML, iconClass;
          if (status === 'success') {
            iconClass = 'bg-success/12';
            iconHTML = '<i data-lucide="check" class="w-3 h-3 text-success"></i>';
          } else if (status === 'error') {
            iconClass = 'bg-error/12';
            iconHTML = '<i data-lucide="x" class="w-3 h-3 text-error"></i>';
          } else {
            iconClass = 'bg-base-200';
            iconHTML = '<span class="loading loading-spinner loading-xs text-primary"></span>';
          }
          iconWrap.className = 'w-6 h-6 rounded-full flex items-center justify-center ' + iconClass;
          iconWrap.innerHTML = iconHTML;
          if (status === 'success' || status === 'error') lucide.createIcons({ nodes: [iconWrap] });
        }

        var dur = row.querySelector('[data-sync-log-duration]');
        if (dur) {
          if (payload.duration_label) {
            dur.textContent = payload.duration_label;
            this._toggleHidden(dur, false);
          } else {
            this._toggleHidden(dur, true);
          }
        }

        var err = row.querySelector('[data-sync-log-error]');
        if (err) {
          if (status === 'error' && (payload.friendly_error_message || payload.error_message)) {
            err.textContent = payload.friendly_error_message || payload.error_message;
            err.title = payload.error_message || '';
            this._toggleHidden(err, false);
          } else {
            this._toggleHidden(err, true);
          }
        }

        var added = payload.added_count || 0;
        var modified = payload.modified_count || 0;
        var removed = payload.removed_count || 0;
        var unchanged = payload.unchanged_count || 0;
        var addedEl = row.querySelector('[data-sync-log-added]');
        var modEl = row.querySelector('[data-sync-log-modified]');
        var remEl = row.querySelector('[data-sync-log-removed]');
        var unchEl = row.querySelector('[data-sync-log-unchanged]');
        var main = row.querySelector('[data-sync-log-counts-main]');
        if (addedEl) { addedEl.textContent = added ? '+' + added : ''; this._toggleHidden(addedEl, !added); }
        if (modEl)   { modEl.textContent   = modified ? '~' + modified : ''; this._toggleHidden(modEl, !modified); }
        if (remEl)   { remEl.textContent   = removed ? '-' + removed : ''; this._toggleHidden(remEl, !removed); }
        if (unchEl)  { unchEl.textContent  = unchanged ? '=' + unchanged : ''; unchEl.title = unchanged + ' unchanged'; this._toggleHidden(unchEl, !unchanged); }
        this._toggleHidden(main, !(added || modified || removed));
      },

      _findPollTargetRow: function () {
        var list = document.getElementById('sync-history-list');
        if (!list) return null;
        var pending = list.querySelector('[data-sync-log-row="pending"]');
        if (pending) return pending;
        var first = list.querySelector('[data-sync-log-row]');
        if (first && first.getAttribute('data-sync-log-status') === 'in_progress') return first;
        return null;
      },

      _stopSyncPoll: function () {
        if (this._syncPollTimer) { clearTimeout(this._syncPollTimer); this._syncPollTimer = null; }
      },

      pollSyncStatus: function () {
        this._stopSyncPoll();
        if (!this._syncPollStartedAt) this._syncPollStartedAt = Date.now();

        var self = this;
        fetch('/-/connections/' + this._connID + '/sync-status', { headers: { 'Accept': 'application/json' } })
          .then(function (res) { return res.ok ? res.json() : null; })
          .then(function (payload) {
            if (!payload || payload.status === 'none') {
              self._syncPollTimer = setTimeout(function () { self.pollSyncStatus(); }, SYNC_POLL_INTERVAL_MS);
              return;
            }

            var row = self._findPollTargetRow();
            if (row) self.updateSyncLogRow(row, payload);

            if (payload.status === 'in_progress') {
              if (Date.now() - self._syncPollStartedAt > SYNC_POLL_MAX_MS) {
                self.showToast('Sync is taking longer than expected — refresh to check status.', 'warning');
                self._syncPollStartedAt = 0;
                self.setSyncButtonIdle();
                return;
              }
              self._syncPollTimer = setTimeout(function () { self.pollSyncStatus(); }, SYNC_POLL_INTERVAL_MS);
              return;
            }

            self._syncPollStartedAt = 0;
            self.setSyncButtonIdle();
            if (payload.status === 'error') {
              self.showToast(payload.friendly_error_message || payload.error_message || 'Sync failed.', 'error');
            }
          })
          .catch(function () {
            self._syncPollTimer = setTimeout(function () { self.pollSyncStatus(); }, SYNC_POLL_INTERVAL_MS);
          });
      },

      // --- Per-connection actions ---
      syncConnection: function () {
        this.setSyncButtonBusy();

        var list = document.getElementById('sync-history-list');
        var empty = document.getElementById('sync-history-empty');
        if (list && !list.querySelector('[data-sync-log-row="pending"]')) {
          var pending = this.buildSyncLogRow({ status: 'in_progress', trigger: 'manual' });
          if (list.firstElementChild) list.insertBefore(pending, list.firstElementChild);
          else list.appendChild(pending);
          this._toggleHidden(list, false);
          this._toggleHidden(empty, true);
        }

        var self = this;
        fetch('/-/connections/' + this._connID + '/sync', { method: 'POST' })
          .then(function (res) {
            if (res.ok) {
              self._syncPollStartedAt = Date.now();
              self.pollSyncStatus();
              return;
            }
            return res.json().then(function (data) {
              self.showToast((data && data.error) || 'Failed to trigger sync.');
              var p = list && list.querySelector('[data-sync-log-row="pending"]');
              if (p) p.remove();
              if (list && !list.firstElementChild) { self._toggleHidden(list, true); self._toggleHidden(empty, false); }
              self.setSyncButtonIdle();
            });
          })
          .catch(function () {
            self.showToast('Network error. Please try again.');
            var p = list && list.querySelector('[data-sync-log-row="pending"]');
            if (p) p.remove();
            if (list && !list.firstElementChild) { self._toggleHidden(list, true); self._toggleHidden(empty, false); }
            self.setSyncButtonIdle();
          });
      },

      removeConnection: function () {
        if (window.bbProgress) window.bbProgress.start();
        var self = this;
        fetch('/-/connections/' + this._connID, { method: 'DELETE' })
          .then(function (res) {
            if (res.ok) {
              if (window.bbProgress) window.bbProgress.finish();
              window.location.href = '/connections';
            } else {
              if (window.bbProgress) window.bbProgress.finish();
              return res.json().then(function (data) {
                self.showToast(data.error || 'Failed to remove connection.');
              });
            }
          })
          .catch(function () {
            if (window.bbProgress) window.bbProgress.finish();
            self.showToast('Network error. Please try again.');
          });
      },

      togglePause: function () {
        var btn = document.getElementById('pause-btn');
        var isPaused = btn.textContent.trim() === 'Resume';
        var newPaused = !isPaused;
        btn.disabled = true;
        var self = this;
        fetch('/-/connections/' + this._connID + '/paused', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ paused: newPaused })
        })
          .then(function (res) {
            if (res.ok) {
              window.location.reload();
            } else {
              return res.json().then(function (data) {
                self.showToast(data.error || 'Failed to update pause state.');
                btn.disabled = false;
              });
            }
          })
          .catch(function () {
            self.showToast('Network error. Please try again.');
            btn.disabled = false;
          });
      },

      updateSyncInterval: function (val) {
        var body = val === '' ? { minutes: null } : { minutes: parseInt(val, 10) };
        var status = document.getElementById('interval-status');
        status.textContent = 'Saving...';
        fetch('/-/connections/' + this._connID + '/sync-interval', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body)
        })
          .then(function (res) {
            if (res.ok) {
              status.textContent = 'Saved';
              setTimeout(function () { status.textContent = ''; }, 2000);
            } else {
              return res.json().then(function (data) {
                status.textContent = data.error || 'Failed';
              });
            }
          })
          .catch(function () {
            status.textContent = 'Network error';
          });
      },

      // --- Per-account actions (called from inline @change handlers on
      // the per-account input + checkbox in the Accounts grid). ---
      updateDisplayName: function (accountId, val) {
        var body = val === '' ? { display_name: null } : { display_name: val };
        var self = this;
        fetch('/-/accounts/' + accountId + '/display-name', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body)
        })
          .then(function (res) {
            if (res.ok) {
              self.showToast('Display name updated.', 'success');
            } else {
              return res.json().then(function (data) {
                self.showToast(data.error || 'Failed to update display name.');
              });
            }
          })
          .catch(function () {
            self.showToast('Network error. Please try again.');
          });
      },

      toggleExcluded: function (accountId, checked) {
        var self = this;
        fetch('/-/accounts/' + accountId + '/excluded', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ excluded: checked })
        })
          .then(function (res) {
            if (res.ok) {
              self.showToast(checked ? 'Account excluded.' : 'Account included.', 'success');
              window.location.reload();
            } else {
              return res.json().then(function (data) {
                self.showToast(data.error || 'Failed to update excluded state.');
              });
            }
          })
          .catch(function () {
            self.showToast('Network error. Please try again.');
          });
      }
    };
  });
});
