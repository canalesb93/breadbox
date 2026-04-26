// Account detail Alpine bridge for /accounts/{id}.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// This page renders no top-level Alpine factory of its own — the page is
// mostly server-rendered + a few inline Alpine x-data sub-blocks (filter
// panel, account-settings disclosure, transaction rows). The role of this
// module is twofold:
//
//   1. Seed `window.__bbCategories` from the @templ.JSONScript payload so
//      the inline `categoryPicker(...)` factory used by tx_row.templ and
//      the filter row's category picker finds a populated category tree
//      on first render. This mirrors the seeding pattern in
//      transaction_detail.js / rule_detail.js.
//
//   2. Expose `showToast`, `quickSetCategory`, `updateDisplayName`, and
//      `toggleExcluded` on `window` so:
//        - the inline `onchange="updateDisplayName(...)"` and
//          `onchange="toggleExcluded(...)"` bindings emitted by
//          acctDisplayNameInputHTML / acctExcludedCheckboxHTML can find them.
//        - tx_row's inline `x-init` watcher and Alpine pickers can call
//          `quickSetCategory` and `showToast`.
//
// A no-op `Alpine.store('bulk', ...)` is registered so tx_row's
// `$store.bulk.toggle()` and `.processing` references don't error on this
// page (we never enter selection mode here).

// --- Module-level globals consumed by tx_row + acct settings inputs ---

function showToast(message, type) {
  window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
}

// Inline category picker on tx-row fires this on change. Same contract as
// /transactions and /rules/{id} — keep in sync if that endpoint evolves.
function quickSetCategory(txId, categoryId) {
  if (!categoryId) {
    fetch('/-/transactions/' + txId + '/category', { method: 'DELETE' })
      .then(function (r) {
        if (r.ok || r.status === 204) showToast('Category reset', 'success');
      });
    return;
  }
  fetch('/-/transactions/' + txId + '/category', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ category_id: categoryId }),
  })
    .then(function (r) {
      if (r.ok) showToast('Category updated', 'success');
      else r.json().then(function (d) { showToast(d.error?.message || 'Failed', 'error'); });
    });
}

// Account-settings: the Display Name input fires onchange; PATCH the
// account and toast on result. Empty string clears the override (sends
// null) so the institution-supplied name takes over.
function updateDisplayName(accountId, val) {
  var body = val === '' ? { display_name: null } : { display_name: val };
  fetch('/-/accounts/' + accountId + '/display-name', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
    .then(function (res) {
      if (res.ok) {
        showToast('Display name updated.', 'success');
      } else {
        return res.json().then(function (data) {
          showToast(data.error || 'Failed to update display name.');
        });
      }
    })
    .catch(function () {
      showToast('Network error. Please try again.');
    });
}

// Account-settings: the Excluded checkbox fires onchange; POST the new
// state and toast on result.
function toggleExcluded(accountId, checked) {
  fetch('/-/accounts/' + accountId + '/excluded', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ excluded: checked }),
  })
    .then(function (res) {
      if (res.ok) {
        showToast(checked ? 'Account excluded.' : 'Account included.', 'success');
      } else {
        return res.json().then(function (data) {
          showToast(data.error || 'Failed to update excluded state.');
        });
      }
    })
    .catch(function () {
      showToast('Network error. Please try again.');
    });
}

window.showToast = showToast;
window.quickSetCategory = quickSetCategory;
window.updateDisplayName = updateDisplayName;
window.toggleExcluded = toggleExcluded;

// Seed window.__bbCategories as early as possible so inline x-data
// initializers (the filter row's categoryPicker, plus tx_row's inline
// categoryPicker on first render) find the populated value. Mirrors the
// IIFE-at-top pattern from transaction_detail.js / rule_detail.js.
//
// The JSONScript tag must be emitted *above* this <script src> in the
// templ component so the element exists when this IIFE runs.
(function seedGlobals() {
  var dataEl = document.getElementById('account-detail-data');
  if (!dataEl) return;
  try {
    window.__bbCategories = JSON.parse(dataEl.textContent);
  } catch (e) {
    console.error('accountDetail: failed to parse #account-detail-data', e);
    window.__bbCategories = null;
  }
})();

document.addEventListener('alpine:init', function () {
  // Minimal no-op bulk store. tx_row references $store.bulk.toggle() and
  // .processing on render; we never enter selection mode on this page,
  // so the stubbed handlers are sufficient.
  if (!Alpine.store('bulk')) {
    Alpine.store('bulk', {
      selecting: false,
      processing: false,
      sel: [],
      isIn: function () { return false; },
      toggle: function () {},
      toggleMode: function () {},
      clear: function () {},
    });
  }
});
