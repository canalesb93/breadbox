// Settings → Notifications — per-channel test-delivery factory.
//
// Powers the "Test" button on each channel row. testState maps a channel id
// to 'loading' | 'ok' | 'err:<message>'; the template reads it to show inline
// feedback. CSRF token comes from a sibling data-* attribute (per
// docs/design-system.md → "Alpine page components").
document.addEventListener('alpine:init', function () {
  Alpine.data('notificationsSettings', function () {
    return {
      csrfToken: '',
      testState: {},

      init: function () {
        this.csrfToken = this.$el.dataset.csrf || '';
      },

      _set: function (id, val) {
        // Reassign so Alpine picks up the new key reactively.
        var next = Object.assign({}, this.testState);
        next[id] = val;
        this.testState = next;
      },

      testChannel: function (id) {
        var self = this;
        this._set(id, 'loading');
        fetch('/-/notifications/channels/' + encodeURIComponent(id) + '/test', {
          method: 'POST',
          credentials: 'same-origin',
          headers: { 'X-CSRF-Token': this.csrfToken, Accept: 'application/json' },
        })
          .then(function (res) {
            return res.json().then(function (body) {
              return { ok: res.ok, status: res.status, body: body };
            });
          })
          .then(function (r) {
            if (!r.ok || (r.body && r.body.ok === false)) {
              self._set(id, 'err:' + ((r.body && r.body.error) || ('HTTP ' + r.status)));
              return;
            }
            self._set(id, 'ok');
          })
          .catch(function (e) {
            self._set(id, 'err:' + ((e && e.message) || 'Request failed'));
          });
      },
    };
  });
});
