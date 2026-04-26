// M3 Reviews queue keyboard shortcuts.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
//
// Loaded only when the transactions page is filtered to tags=needs-review
// (see txReviewQueueShortcuts in transactions.templ). Registers the single
// review-specific action - approve (a) - at `scope: 'reviews'` on top of the
// transactions list's existing j/k/Enter/c handlers (registered by
// transactions.js when the same page boots).
//
// Reject and skip were considered but dropped: the review model is tag-backed
// (the needs-review tag is the queue), so there is no distinct "rejected"
// state to record - reject would be semantically identical to approve with
// different toast copy. Skip, without a tracked snooze-until mechanism, is
// just `j` (advance focus). We keep the transactions list free of custom
// logic that doesn't pull its weight.
//
// Guards (input focus, open overlays, touch devices) are owned by the global
// dispatcher in base.html, so these handlers only need to care about "what
// does this key do" when it fires.
(function () {
  function focusedRow() {
    var nav = window.Alpine && Alpine.store('txNav');
    if (!nav) return null;
    // Private helper on the store - the store exposes it via `_getRows`
    // for j/k nav. Fall back to a DOM query if the store shape drifts.
    var rows = typeof nav._getRows === 'function'
      ? nav._getRows()
      : Array.prototype.slice.call(document.querySelectorAll('.bb-tx-row'));
    if (nav.focusedIdx < 0 || nav.focusedIdx >= rows.length) return null;
    return rows[nav.focusedIdx];
  }

  function advanceFocus() {
    var nav = window.Alpine && Alpine.store('txNav');
    if (!nav) return;
    nav.next();
  }

  // Resolve the needs-review tag on the focused row. Calls the same DELETE
  // endpoint the tag chip's remove button uses so CSRF, annotations, and
  // any other server-side side effects stay in one code path. On success,
  // the row is removed from the view (mirrors removeTag's animation in
  // transactions.js) and focus advances to the next row.
  function resolveReview(verb) {
    var row = focusedRow();
    if (!row) return;
    var txId = row.dataset.txId;
    if (!txId) return;
    fetch('/-/transactions/' + encodeURIComponent(txId) + '/tags/needs-review', {
      method: 'DELETE',
      headers: { 'Accept': 'application/json' }
    })
      .then(function (r) { return r.json().catch(function () { return {}; }); })
      .then(function (d) {
        if (d && (d.ok || d.removed)) {
          // Advance focus BEFORE the row is gone so the focus ring lands on
          // the row that slides up into its slot. txNav.next() is idempotent
          // if already at the end, so no special-case for the last review.
          advanceFocus();
          row.style.transition = 'opacity 150ms';
          row.style.opacity = '0';
          setTimeout(function () { row.remove(); }, 160);
          window.dispatchEvent(new CustomEvent('bb-toast', {
            detail: { message: 'Review ' + verb, type: 'success' }
          }));
        } else {
          window.dispatchEvent(new CustomEvent('bb-toast', {
            detail: { message: (d && d.error) || 'Failed to resolve review', type: 'error' }
          }));
        }
      })
      .catch(function () {
        window.dispatchEvent(new CustomEvent('bb-toast', {
          detail: { message: 'Failed to resolve review', type: 'error' }
        }));
      });
  }

  function register() {
    var reg = window.Alpine && Alpine.store('shortcuts');
    if (!reg) return;

    reg.register({
      id: 'reviews.approve',
      keys: 'a',
      description: 'Approve review',
      group: 'Actions',
      scope: 'reviews',
      action: function () { resolveReview('approved'); }
    });
  }

  if (window.Alpine) {
    register();
  } else {
    document.addEventListener('alpine:init', register);
  }
})();
