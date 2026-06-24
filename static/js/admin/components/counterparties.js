// Counterparties list page Alpine component for /counterparties.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// A counterparty is a thin, rule-maintained entity with no detector, so the list
// is a flat directory with a single client-side filter: a free-text search over
// the row haystack. Mirrors subscriptionsList (the /recurring ledger). Also
// scopes the keyboard shortcuts to this page while mounted.
(function () {
  document.addEventListener('alpine:init', function () {
    Alpine.data('counterpartiesList', function () {
      return {
        q: '',

        init: function () {
          var reg = window.Alpine && Alpine.store('shortcuts');
          if (reg) reg.setScope('counterparties');
        },

        destroy: function () {
          var reg = window.Alpine && Alpine.store('shortcuts');
          if (reg) reg.setScope('global');
        },

        // Search filter for the directory rows (data-search haystack).
        matches: function (el) {
          if (!this.q) return true;
          return (el.dataset.search || '').indexOf(this.q.toLowerCase()) !== -1;
        },
      };
    });
  });
})();
