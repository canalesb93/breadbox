// Accounts list page Alpine component for /accounts.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Owns the client-side filter (search input + family-member chip strip)
// and sort state for the flat accounts table.
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
    Alpine.data('accountsList', function () {
      return {
        filter: '',
        filterUser: 'all',
        sortKey: 'balance',
        sortDir: 'desc',

        init: function () {
          var self = this;
          // Listen for excluded-toggle dispatches from the row overflow menu.
          window.addEventListener('account-set-excluded', function (e) {
            self.setExcluded(e.detail.id, e.detail.excluded);
          });
          // Initial sort on mount so SSR-ordered rows respect the default key.
          this.applySort();
        },

        matches: function (el) {
          if (!this.filter) return true;
          return (el.dataset.search || '').includes(this.filter.toLowerCase());
        },

        setSort: function (key) {
          if (this.sortKey === key) {
            this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc';
          } else {
            this.sortKey = key;
            // Balance defaults to descending (largest first); text columns ascending.
            this.sortDir = key === 'balance' ? 'desc' : 'asc';
          }
          this.applySort();
        },

        applySort: function () {
          var tbody = document.querySelector('[data-account-row]');
          if (!tbody) return;
          tbody = tbody.parentElement;
          if (!tbody) return;
          var rows = Array.prototype.slice.call(tbody.querySelectorAll('[data-account-row]'));
          var key = this.sortKey;
          var dir = this.sortDir === 'asc' ? 1 : -1;
          rows.sort(function (a, b) {
            var av, bv;
            if (key === 'balance') {
              // Rows without a balance sink to the bottom regardless of direction.
              var aHas = a.dataset.hasBalance === '1';
              var bHas = b.dataset.hasBalance === '1';
              if (aHas !== bHas) return aHas ? -1 : 1;
              av = parseFloat(a.dataset.balance) || 0;
              bv = parseFloat(b.dataset.balance) || 0;
              return (av - bv) * dir;
            }
            av = (a.dataset[key] || '');
            bv = (b.dataset[key] || '');
            if (av < bv) return -1 * dir;
            if (av > bv) return 1 * dir;
            return 0;
          });
          for (var i = 0; i < rows.length; i++) tbody.appendChild(rows[i]);
        },

        setExcluded: function (accountId, excluded) {
          fetch('/-/accounts/' + accountId + '/excluded', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ excluded: excluded })
          })
            .then(function (res) {
              if (res.ok) {
                toast(excluded ? 'Account excluded from totals.' : 'Account included in totals.', 'success');
                window.location.reload();
              } else {
                restorePageState();
                return res.json().then(function (data) {
                  toast((data.error && data.error.message) || data.error || 'Failed to update account.');
                });
              }
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
