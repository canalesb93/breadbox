// Settings page scripts for /settings.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// Two pieces of behavior live here:
//
//   1. Hash auto-scroll — if the URL has a fragment (#sync, #retention,
//      #security, #help, …) scroll the matching element into view after
//      a short delay so the modal's body has time to render.
//
//   2. `settingsAutoSave` Alpine factory — submits the wrapper <form>
//      on every `change` event from a child control. The Settings modal
//      already intercepts POSTs from within #bb-settings-body, so this
//      factory only has to fire `requestSubmit()`. The form's
//      data-toast-label attribute is the label the modal shows when the
//      server response doesn't ship its own flash.
//
// The <script src> loads synchronously at the top of the templ component
// (so any future alpine:init listeners register before Alpine fires the
// event). That means this script runs BEFORE the rest of the body is
// parsed, so the hash-scroll function has to wait for DOMContentLoaded to
// query the target element.
function bbSettingsScrollToHash() {
  if (!location.hash) return;
  var el = document.querySelector(location.hash);
  if (el) {
    setTimeout(function () {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }, 100);
  }
}
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bbSettingsScrollToHash);
} else {
  bbSettingsScrollToHash();
}

document.addEventListener('alpine:init', function () {
  Alpine.data('settingsAutoSave', function () {
    return {
      _saveTimer: null,

      init: function () {
        var self = this;
        // Listen for `change` on every input/select/checkbox descendant.
        // `change` fires after the user commits the new value (after
        // dropdown close or blur for text inputs), which is the right
        // semantic for "save now."
        this.$el.addEventListener('change', function (e) {
          var target = e.target;
          if (!target || target.tagName === 'BUTTON') return;
          // Skip the hidden CSRF input and any control marked as a
          // helper (e.g. preview pickers that should not save).
          if (target.type === 'hidden') return;
          if (target.dataset.autosaveSkip === 'true') return;
          // Coalesce rapid changes (e.g. multiple toggles) into one
          // submit per ~120ms. Not strictly needed for selects, but
          // costs nothing and protects future checkbox callers.
          clearTimeout(self._saveTimer);
          self._saveTimer = setTimeout(function () {
            try { self.$el.requestSubmit(); } catch (err) {
              // Older browsers without requestSubmit fall back to a
              // synchronous submit — still gets picked up by the
              // modal's submit interceptor.
              self.$el.submit();
            }
          }, 120);
        });
      },
    };
  });
});
