// Recurring-series detail page (/recurring/{id}) Alpine component — the
// rules-as-substrate model.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
//
// A series is a THIN entity (a name + a type) whose membership is defined by the
// `assign_series` rules that target it. This component covers the small set of
// direct edits the detail page still exposes:
//   - DRAWER + explicit Save: name + type -> PATCH /api/v1/series/{id}.
//   - Tags: inline add/remove via the shared tag picker -> series tag endpoints.
//   - Linked charges: per-row unlink (DELETE) + a "Link a charge" search modal.
//
// Every write reloads on success so the server re-renders the linked charges and
// governing-rules panels. CSRF is auto-injected by the global fetch wrapper.
(function () {
  function restorePageState() {
    if (window.bbProgress && window.bbProgress.finish) window.bbProgress.finish();
    var main = document.querySelector('main');
    if (main) {
      main.style.opacity = '';
      main.style.filter = '';
      main.style.pointerEvents = '';
    }
  }

  function toast(message, type) {
    window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
  }

  function parseJSONScript(id, fallback) {
    var el = document.getElementById(id);
    if (!el) return fallback;
    try { return JSON.parse(el.textContent) || fallback; } catch (e) { return fallback; }
  }

  // Seed the globals the shared tag picker reads on first render.
  (function seedGlobals() {
    window.__bbAllTags = parseJSONScript('series-detail-alltags', window.__bbAllTags || []);
  })();

  document.addEventListener('alpine:init', function () {
    Alpine.data('seriesDetail', function () {
      return {
        seriesId: '',
        currentTags: [],
        editOpen: false,
        // Link-a-charge modal state.
        linkOpen: false,
        linkQuery: '',
        linkResults: [],
        linkLoading: false,
        _linkAbort: null,

        init: function () {
          this.seriesId = (this.$root && this.$root.dataset.seriesId) || '';
          this.currentTags = parseJSONScript('series-detail-current-tags', []);
        },

        // --- Generic writer: mutate then reload on success ------------------
        _write: function (method, url, body, okMsg, failMsg) {
          var opts = { method: method, headers: { Accept: 'application/json' } };
          if (body) {
            opts.headers['Content-Type'] = 'application/json';
            opts.body = JSON.stringify(body);
          }
          return fetch(url, opts).then(function (res) {
            if (res.ok) {
              toast(okMsg, 'success');
              window.location.reload();
              return true;
            }
            restorePageState();
            return res.json().then(function (data) {
              toast((data.error && data.error.message) || failMsg);
            }).catch(function () { toast(failMsg); }).then(function () { return false; });
          }).catch(function () { restorePageState(); toast('Network error. Please try again.'); return false; });
        },

        // --- Drawer: edit the two thin attributes (name + type), one Save ---
        saveDrawer: function (form) {
          var f = new FormData(form);
          var body = { name: (f.get('name') || '').trim(), type: f.get('type') };
          if (!body.name) { toast('Name cannot be empty.'); return; }
          this._write('PATCH', '/api/v1/series/' + encodeURIComponent(this.seriesId), body, 'Saved', 'Failed to save.');
        },

        // --- Tags: shared picker + per-chip remove --------------------------
        openTagPicker: function () {
          var counts = {};
          this.currentTags.forEach(function (slug) { counts[slug] = 1; });
          window.dispatchEvent(new CustomEvent('open-tag-picker', {
            detail: {
              sourceId: 'series-tag',
              transactionIds: [],
              txCount: 0,
              appliedCounts: counts,
              availableTags: window.__bbAllTags || [],
            },
          }));
        },

        // Apply the picker's add/remove diff via the series tag endpoints, then
        // reload once (so members re-inherit / shed the tags).
        applyTagDiff: function (adds, removes) {
          var self = this;
          var ops = [];
          (adds || []).forEach(function (slug) {
            ops.push(self._tagOp('POST', '/api/v1/series/' + encodeURIComponent(self.seriesId) + '/tags', { tag_slug: slug }));
          });
          (removes || []).forEach(function (slug) {
            ops.push(self._tagOp('DELETE', '/api/v1/series/' + encodeURIComponent(self.seriesId) + '/tags/' + encodeURIComponent(slug), null));
          });
          if (ops.length === 0) return;
          Promise.all(ops).then(function () {
            toast('Tags updated', 'success');
            window.location.reload();
          }).catch(function () { restorePageState(); toast('Failed to update tags.'); });
        },

        removeSeriesTag: function (seriesId, slug) {
          this._write('DELETE', '/api/v1/series/' + encodeURIComponent(seriesId) + '/tags/' + encodeURIComponent(slug), null, 'Tag removed', 'Failed to remove tag.');
        },

        _tagOp: function (method, url, body) {
          var opts = { method: method, headers: { Accept: 'application/json' } };
          if (body) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
          return fetch(url, opts).then(function (res) { if (!res.ok) throw new Error('tag op failed'); });
        },

        // --- Member charges -------------------------------------------------
        unlinkCharge: function (seriesId, txId) {
          this._write('DELETE', '/api/v1/series/' + encodeURIComponent(seriesId) + '/transactions/' + encodeURIComponent(txId), null, 'Charge unlinked', 'Failed to unlink charge.');
        },

        linkCharge: function (txId) {
          this._write('POST', '/api/v1/series/' + encodeURIComponent(this.seriesId) + '/transactions', { transaction_ids: [txId] }, 'Charge linked', 'Failed to link charge.');
        },

        openLink: function () { this.linkOpen = true; this.linkQuery = ''; this.linkResults = []; this.$nextTick(function () { var i = document.getElementById('series-link-search'); if (i) i.focus(); }); },
        closeLink: function () { this.linkOpen = false; },

        searchLink: function () {
          var self = this;
          var q = (this.linkQuery || '').trim();
          if (q.length < 2) { this.linkResults = []; this.linkLoading = false; return; }
          this.linkLoading = true;
          if (this._linkAbort) this._linkAbort.abort();
          this._linkAbort = new AbortController();
          fetch('/-/search/transactions?q=' + encodeURIComponent(q), { headers: { Accept: 'application/json' }, signal: this._linkAbort.signal })
            .then(function (res) { return res.json(); })
            .then(function (items) { self.linkResults = items || []; self.linkLoading = false; })
            .catch(function (err) { if (err.name !== 'AbortError') { self.linkLoading = false; } });
        },
      };
    });
  });
})();
