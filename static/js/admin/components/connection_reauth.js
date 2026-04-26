// Connection re-authentication Alpine component for /connections/{id}/reauth.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Initial state is read from data-* attributes on the root x-data element:
//   data-conn-id, data-provider, data-teller-app-id, data-teller-env.
//
// The component bootstraps Plaid Link or Teller Connect on load, posts the
// resulting public_token / enrollment back to the server, and redirects to
// the connection detail page on success.
document.addEventListener('alpine:init', function () {
  Alpine.data('connectionReauth', function () {
    return {
      connId: '',
      provider: '',
      tellerAppId: '',
      tellerEnv: '',
      status: 'Opening re-authentication dialog...',
      isError: false,
      canRetry: false,

      init: function () {
        var root = this.$el;
        this.connId = root.dataset.connId || '';
        this.provider = root.dataset.provider || '';
        this.tellerAppId = root.dataset.tellerAppId || '';
        this.tellerEnv = root.dataset.tellerEnv || '';
        this.startReauth();
      },

      showError: function (msg) {
        this.status = msg;
        this.isError = true;
        this.canRetry = true;
      },

      showMessage: function (msg) {
        this.status = msg;
        this.isError = false;
      },

      startReauth: function () {
        this.showMessage('Opening re-authentication dialog...');
        this.canRetry = false;

        var self = this;
        fetch('/-/connections/' + this.connId + '/reauth', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
        })
          .then(function (res) { return res.json(); })
          .then(function (data) {
            if (data.link_token) {
              if (self.provider === 'teller') {
                self.initTellerReauth(data.link_token);
              } else {
                self.initPlaidReauth(data.link_token);
              }
            } else {
              self.showError(data.error || 'Failed to initialize re-authentication.');
            }
          })
          .catch(function () {
            self.showError('Network error. Please try again.');
          });
      },

      initPlaidReauth: function (token) {
        var self = this;
        var handler = Plaid.create({
          token: token,
          onSuccess: function (publicToken) {
            self.showMessage('Completing re-authentication...');
            fetch('/-/connections/' + self.connId + '/reauth-complete', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ public_token: publicToken }),
            })
              .then(function (res) { return res.json(); })
              .then(function (data) {
                if (data.status === 'active') {
                  window.location.href = '/connections/' + self.connId;
                } else {
                  self.showError(data.error || 'Re-authentication failed.');
                }
              })
              .catch(function () {
                self.showError('Network error. Please try again.');
              });
          },
          onExit: function (err) {
            if (err) {
              self.showError('Re-authentication exited with an error: ' + (err.display_message || err.error_message || 'Unknown error'));
            } else {
              self.showMessage('Re-authentication cancelled.');
              self.canRetry = true;
            }
          },
        });
        handler.open();
      },

      initTellerReauth: function (enrollmentId) {
        var self = this;
        var tellerConnect = TellerConnect.setup({
          applicationId: this.tellerAppId,
          enrollmentId: enrollmentId,
          environment: this.tellerEnv,
          onSuccess: function () {
            self.showMessage('Completing re-authentication...');
            fetch('/-/connections/' + self.connId + '/reauth-complete', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
            })
              .then(function (res) { return res.json(); })
              .then(function (data) {
                if (data.status === 'active') {
                  window.location.href = '/connections/' + self.connId;
                } else {
                  self.showError(data.error || 'Re-authentication failed.');
                }
              })
              .catch(function () {
                self.showError('Network error. Please try again.');
              });
          },
          onExit: function () {
            self.showMessage('Re-authentication cancelled.');
            self.canRetry = true;
          },
        });
        tellerConnect.open();
      },
    };
  });
});
