// Provider card Alpine component for /providers.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Each card (Plaid, Teller) instantiates its own copy via x-data="providerCard"
// and supplies the provider name as a `data-provider` attribute on the root,
// read inside testConnection() via this.$el.dataset.provider. Keeping the
// factory argument-free matches the convention; the per-instance variation
// flows through the DOM, not through factory args.
document.addEventListener('alpine:init', function () {
  Alpine.data('providerCard', function () {
    return {
      showConfig: false,
      saving: false,
      testing: false,
      testResult: '',
      testSuccess: false,

      testConnection: function () {
        var self = this;
        var name = this.$el.dataset.provider;
        this.testing = true;
        this.testResult = '';
        fetch('/-/test-provider/' + name, { method: 'POST' })
          .then(function (res) { return res.json(); })
          .then(function (data) {
            self.testing = false;
            self.testSuccess = data.success;
            self.testResult = data.message || data.error || 'Unknown result';
            setTimeout(function () { self.testResult = ''; }, 8000);
          })
          .catch(function () {
            self.testing = false;
            self.testSuccess = false;
            self.testResult = 'Network error';
            setTimeout(function () { self.testResult = ''; }, 8000);
          });
      }
    };
  });
});
