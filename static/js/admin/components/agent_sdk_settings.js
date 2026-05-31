// Agent SDK settings — Diagnostics card factory.
//
// Powers the "Run smoke test" + "Run cleanup now" buttons on
// /settings/agents. Reads its CSRF token from a sibling data-* attribute on
// the x-data root so the factory itself takes no arguments (per the
// docs/design-system.md → "Alpine page components" convention).
document.addEventListener('alpine:init', function () {
  Alpine.data('agentSDKDiagnostics', function () {
    return {
      csrfToken: '',
      smokeState: 'idle',
      smokeResult: null,
      smokeError: null,
      cleanupState: 'idle',
      cleanupResult: null,
      cleanupError: null,
      notifyState: 'idle',
      notifyError: null,

      init: function () {
        this.csrfToken = this.$el.dataset.csrf || '';
      },

      _post: function (url) {
        return fetch(url, {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'X-CSRF-Token': this.csrfToken,
            Accept: 'application/json',
          },
        });
      },

      runSmokeTest: function () {
        var self = this;
        this.smokeState = 'loading';
        this.smokeResult = null;
        this.smokeError = null;
        this._post('/-/agents/test')
          .then(function (res) {
            return res.json().then(function (body) {
              return { ok: res.ok, status: res.status, body: body };
            });
          })
          .then(function (r) {
            if (!r.ok) {
              self.smokeError = (r.body && r.body.error && r.body.error.message) || ('HTTP ' + r.status);
              self.smokeState = 'error';
              return;
            }
            self.smokeResult = r.body;
            self.smokeState = 'ok';
          })
          .catch(function (e) {
            self.smokeError = (e && e.message) || 'Request failed';
            self.smokeState = 'error';
          });
      },

      runCleanup: function () {
        var self = this;
        this.cleanupState = 'loading';
        this.cleanupResult = null;
        this.cleanupError = null;
        this._post('/-/agents/cleanup')
          .then(function (res) {
            return res.json().then(function (body) {
              return { ok: res.ok, status: res.status, body: body };
            });
          })
          .then(function (r) {
            if (!r.ok) {
              self.cleanupError = (r.body && r.body.error && r.body.error.message) || ('HTTP ' + r.status);
              self.cleanupState = 'error';
              return;
            }
            self.cleanupResult = r.body;
            self.cleanupState = 'ok';
          })
          .catch(function (e) {
            self.cleanupError = (e && e.message) || 'Request failed';
            self.cleanupState = 'error';
          });
      },

      runNotifyTest: function () {
        var self = this;
        this.notifyState = 'loading';
        this.notifyError = null;
        this._post('/-/agents/notify-test')
          .then(function (res) {
            return res.json().then(function (body) {
              return { ok: res.ok, status: res.status, body: body };
            });
          })
          .then(function (r) {
            if (!r.ok || (r.body && r.body.ok === false)) {
              self.notifyError = (r.body && r.body.error) || ('HTTP ' + r.status);
              self.notifyState = 'error';
              return;
            }
            self.notifyState = 'ok';
          })
          .catch(function (e) {
            self.notifyError = (e && e.message) || 'Request failed';
            self.notifyState = 'error';
          });
      },
    };
  });
});
