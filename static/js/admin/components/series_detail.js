// Recurring-series detail page (/recurring/{id}) Alpine component.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
//
// Owns every mutation on the detail page: inline field edits (name, amount,
// cadence, expected day, tolerance, category) via PATCH /api/v1/series/{id};
// the type axis via POST /api/v1/series/{id}/type; tags via the series tag
// endpoints; lifecycle verdicts; and link/unlink of member charges (the latter
// powered by a search modal over /-/search/transactions). Every write reloads
// on success so the server re-derives dependent fields (next renewal, renewal
// health, rollups, price history) — correctness over a no-reload flourish.
// CSRF is auto-injected by the global fetch wrapper in base.html.
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

  document.addEventListener('alpine:init', function () {
    Alpine.data('seriesDetail', function () {
      return {
        seriesId: '',
        // Link-a-charge modal state.
        linkOpen: false,
        linkQuery: '',
        linkResults: [],
        linkLoading: false,
        _linkAbort: null,

        init: function () {
          this.seriesId = (this.$root && this.$root.dataset.seriesId) || '';
        },

        // --- Generic writers -------------------------------------------------

        // _write fires a mutation and reloads on success; on failure it restores
        // page chrome and surfaces the server's error message.
        _write: function (method, url, body, okMsg, failMsg) {
          var opts = { method: method, headers: { Accept: 'application/json' } };
          if (body) {
            opts.headers['Content-Type'] = 'application/json';
            opts.body = JSON.stringify(body);
          }
          fetch(url, opts)
            .then(function (res) {
              if (res.ok) {
                toast(okMsg, 'success');
                window.location.reload();
                return;
              }
              restorePageState();
              return res.json().then(function (data) {
                toast((data.error && data.error.message) || failMsg);
              }).catch(function () { toast(failMsg); });
            })
            .catch(function () { restorePageState(); toast('Network error. Please try again.'); });
        },

        patch: function (seriesId, body, okMsg) {
          this._write('PATCH', '/api/v1/series/' + encodeURIComponent(seriesId), body, okMsg || 'Saved', 'Failed to save.');
        },

        // --- Field editors ---------------------------------------------------

        saveName: function (seriesId, value) {
          var v = (value || '').trim();
          if (!v) { toast('Name cannot be empty.'); return; }
          this.patch(seriesId, { name: v }, 'Name updated');
        },

        saveCadence: function (seriesId, value) {
          this.patch(seriesId, { cadence: value }, 'Cadence updated');
        },

        saveExpectedDay: function (seriesId, value) {
          var n = parseInt(value, 10);
          if (isNaN(n)) { toast('Expected day must be a number.'); return; }
          this.patch(seriesId, { expected_day: n }, 'Expected day updated');
        },

        saveAmount: function (seriesId, amount, currency) {
          var n = parseFloat(amount);
          if (isNaN(n)) { toast('Amount must be a number.'); return; }
          var body = { expected_amount: n };
          if (currency) body.currency = currency;
          this.patch(seriesId, body, 'Expected amount updated');
        },

        saveTolerance: function (seriesId, value) {
          var n = parseFloat(value);
          if (isNaN(n)) { toast('Tolerance must be a number.'); return; }
          this.patch(seriesId, { amount_tolerance: n }, 'Tolerance updated');
        },

        saveCategory: function (seriesId, value) {
          // "" clears the suggested category.
          this.patch(seriesId, { category_id: value }, 'Category updated');
        },

        setType: function (seriesId, value) {
          this._write('POST', '/api/v1/series/' + encodeURIComponent(seriesId) + '/type', { type: value }, 'Type updated', 'Failed to set type.');
        },

        // --- Verdicts --------------------------------------------------------

        submitVerdict: function (seriesId, seriesName, verdict) {
          var label = { confirm: 'Confirmed', reject: 'Marked not recurring', pause: 'Paused', cancel: 'Cancelled' }[verdict] || 'Updated';
          this._write('PATCH', '/api/v1/series/' + encodeURIComponent(seriesId), { verdict: verdict }, label + (seriesName ? ': ' + seriesName : ''), 'Failed to update recurring charge.');
        },

        // --- Tags ------------------------------------------------------------

        addSeriesTag: function (seriesId, slug) {
          if (!slug) return;
          this._write('POST', '/api/v1/series/' + encodeURIComponent(seriesId) + '/tags', { tag_slug: slug }, 'Tag added', 'Failed to add tag.');
        },

        removeSeriesTag: function (seriesId, slug) {
          this._write('DELETE', '/api/v1/series/' + encodeURIComponent(seriesId) + '/tags/' + encodeURIComponent(slug), null, 'Tag removed', 'Failed to remove tag.');
        },

        // --- Member charges --------------------------------------------------

        unlinkCharge: function (seriesId, txId) {
          this._write('DELETE', '/api/v1/series/' + encodeURIComponent(seriesId) + '/transactions/' + encodeURIComponent(txId), null, 'Charge unlinked', 'Failed to unlink charge.');
        },

        linkCharge: function (txId) {
          this._write('POST', '/api/v1/series/' + encodeURIComponent(this.seriesId) + '/transactions', { transaction_ids: [txId] }, 'Charge linked', 'Failed to link charge.');
        },

        // Link-a-charge modal: debounced search reusing the /-/search/transactions
        // endpoint (returns pre-rendered TxRowCompact HTML the cmdk palette uses).
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
