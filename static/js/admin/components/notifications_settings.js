// Settings → Notifications — test-delivery button factory.
//
// Powers the "Send test" button on /settings/notifications. Reads its CSRF
// token from a sibling data-* attribute on the x-data root so the factory
// itself takes no arguments (per docs/design-system.md → "Alpine page
// components").
document.addEventListener('alpine:init', function () {
  Alpine.data('notificationsSettings', function () {
    return {
      csrfToken: '',
      state: 'idle',
      error: null,

      init: function () {
        this.csrfToken = this.$el.dataset.csrf || '';
      },

      runTest: function () {
        var self = this;
        this.state = 'loading';
        this.error = null;
        fetch('/-/notifications/test', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'X-CSRF-Token': this.csrfToken,
            Accept: 'application/json',
          },
        })
          .then(function (res) {
            return res.json().then(function (body) {
              return { ok: res.ok, status: res.status, body: body };
            });
          })
          .then(function (r) {
            if (!r.ok || (r.body && r.body.ok === false)) {
              self.error = (r.body && r.body.error) || ('HTTP ' + r.status);
              self.state = 'error';
              return;
            }
            self.state = 'ok';
          })
          .catch(function (e) {
            self.error = (e && e.message) || 'Request failed';
            self.state = 'error';
          });
      },
    };
  });
});
