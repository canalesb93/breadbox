// Access tab scripts for /settings/api-keys.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// `accessReveal` powers the one-time secret block shown right after a key
// is minted. Its job is to let the user *name the key in the same breath as
// copying the secret*: the Name field is pre-focused with its text selected,
// so a single keystroke renames the just-created "API key" default.
//
// The save is a background fetch, never a form submit — the settings shell
// swaps the whole body on any in-tab POST, which would tear down the reveal
// (and the one-shot secret) before the user copied it. The fetch carries
// `X-Requested-With: fetch`; the rename handler answers 204 so nothing
// re-renders and the secret stays put.
//
// Implementation note: `save` closes over `self` (captured in `init`, the
// one lifecycle hook where `this` is reliably the component) rather than
// reading `this`. Alpine invokes `@change="save()"` as a bare call, so the
// method's own `this` isn't guaranteed to be the reactive data — `self`
// sidesteps that entirely.
document.addEventListener('alpine:init', function () {
  Alpine.data('accessReveal', function () {
    var self;
    return {
      label: '',
      saving: false,
      saved: false,

      init: function () {
        self = this;
        this.label = this.$el.dataset.initialLabel || '';
        // Focus + select-all so the default name is one keystroke from gone.
        this.$nextTick(function () {
          var input = self.$refs.labelInput;
          if (input) {
            input.focus();
            input.select();
          }
        });
      },

      // save persists the current label to the rename endpoint. Fires on
      // `change` (commit-on-blur) so we don't POST on every keystroke. Empty
      // names are ignored — the server keeps the "API key" default.
      save: function () {
        var name = (self.label || '').trim();
        if (!name) return;
        self.saving = true;
        self.saved = false;
        fetch(self.$el.dataset.renameUrl, {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'X-Requested-With': 'fetch',
            'Content-Type': 'application/x-www-form-urlencoded',
          },
          body: new URLSearchParams({
            _csrf: self.$el.dataset.csrf || '',
            name: name,
          }).toString(),
        }).then(function (res) {
          self.saving = false;
          self.saved = res.ok;
        }).catch(function () {
          self.saving = false;
        });
      },
    };
  });
});
