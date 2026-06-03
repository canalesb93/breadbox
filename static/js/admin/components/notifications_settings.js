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
      allState: 'idle',

      init: function () {
        this.csrfToken = this.$el.dataset.csrf || '';
      },

      _runTest: function (url, set) {
        set('loading');
        fetch(url, {
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
              set('err:' + ((r.body && r.body.error) || ('HTTP ' + r.status)));
              return;
            }
            set('ok');
          })
          .catch(function (e) {
            set('err:' + ((e && e.message) || 'Request failed'));
          });
      },

      testAll: function () {
        var self = this;
        this._runTest('/-/notifications/test', function (v) {
          self.allState = v;
        });
      },

      _set: function (id, val) {
        // Reassign so Alpine picks up the new key reactively.
        var next = Object.assign({}, this.testState);
        next[id] = val;
        this.testState = next;
      },

      testChannel: function (id) {
        var self = this;
        this._runTest('/-/notifications/channels/' + encodeURIComponent(id) + '/test', function (v) {
          self._set(id, v);
        });
      },
    };
  });
});
