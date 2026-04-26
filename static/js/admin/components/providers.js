// Provider card Alpine component for /providers.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Each card (Plaid, Teller) instantiates its own copy via x-data="providerCard"
// and supplies the provider name as a `data-provider` attribute on the root.
// We read the attribute once in init() — `this.$el` only points at the
// x-data root inside init(); inside event-bound methods (e.g.
// testConnection() called from @click) Alpine binds `this.$el` to the
// element with the directive (the button), which has no data-provider.
document.addEventListener('alpine:init', function () {
  Alpine.data('providerCard', function () {
    return {
      provider: '',
      showConfig: false,
      saving: false,
      testing: false,
      testResult: '',
      testSuccess: false,

      init: function () {
        this.provider = this.$el.dataset.provider || '';
      },

      testConnection: function () {
        var self = this;
        this.testing = true;
        this.testResult = '';
        fetch('/-/test-provider/' + this.provider, { method: 'POST' })
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
