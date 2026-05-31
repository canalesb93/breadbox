// Subscriptions page Alpine component for /subscriptions and
// /subscriptions/{id}.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
//
// Owns the household-member filter + search filter on the list, and the
// verdict actions (confirm / reject / pause / cancel) on both list cards and
// the detail toolbar. Verdicts go to PATCH /api/v1/series/{id} — the same
// REST endpoint the review_series MCP tool wraps. CSRF is auto-injected by the
// global fetch wrapper in base.html, so we don't set the header here.
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
    Alpine.data('subscriptionsList', function () {
      return {
        filterUser: 'all',
        filter: '',

        init: function () {
          var self = this;
          window.addEventListener('series-verdict', function (e) {
            self.submitVerdict(e.detail.id, e.detail.name, e.detail.verdict);
          });
        },

        // Search filter for the active-ledger rows (data-search haystack).
        matches: function (el) {
          if (!this.filter) return true;
          return (el.dataset.search || '').indexOf(this.filter.toLowerCase()) !== -1;
        },

        submitVerdict: function (seriesId, seriesName, verdict) {
          var label = {
            confirm: 'Confirmed',
            reject: 'Marked not recurring',
            pause: 'Paused',
            cancel: 'Cancelled',
          }[verdict] || 'Updated';

          fetch('/api/v1/series/' + encodeURIComponent(seriesId), {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
            body: JSON.stringify({ verdict: verdict }),
          })
            .then(function (res) {
              if (res.ok) {
                toast(label + (seriesName ? ': ' + seriesName : ''), 'success');
                window.location.reload();
                return;
              }
              restorePageState();
              return res.json().then(function (data) {
                toast((data.error && data.error.message) || 'Failed to update recurring charge.');
              }).catch(function () {
                toast('Failed to update recurring charge.');
              });
            })
            .catch(function () {
              restorePageState();
              toast('Network error. Please try again.');
            });
        },
      };
    });
  });
})();
