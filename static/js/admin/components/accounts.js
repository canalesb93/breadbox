// Accounts list page Alpine component for /accounts.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// The page groups accounts by connection server-side and the member filter is
// a server-side dropdown, so the only client behaviour left is the
// exclude/include action dispatched from a row's overflow menu.
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
        init: function () {
          var self = this;
          // Listen for excluded-toggle dispatches from the row overflow menu.
          window.addEventListener('account-set-excluded', function (e) {
            self.setExcluded(e.detail.id, e.detail.excluded);
          });
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
