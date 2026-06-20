// Recurring list page Alpine component for /recurring.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
//
// Under the rules-as-substrate model a series is a thin entity with no detector,
// so the list is a flat ledger with just two client-side filters: type and a
// free-text search over the row haystack. No verdicts, no status grouping.
(function () {
  document.addEventListener('alpine:init', function () {
    Alpine.data('subscriptionsList', function () {
      return {
        filterType: 'all',
        filter: '',

        // Search filter for the ledger rows (data-search haystack).
        matches: function (el) {
          if (!this.filter) return true;
          return (el.dataset.search || '').indexOf(this.filter.toLowerCase()) !== -1;
        },
      };
    });
  });
})();
