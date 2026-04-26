// Tags page Alpine component for /tags.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Owns the search filter state and the per-row delete flow (confirm → DELETE
// fetch → toast + reload). The factory takes no arguments — row metadata
// (id, slug, transaction count) flows in through the deleteTag() call site
// via dataset attributes on the kebab Delete button.
document.addEventListener('alpine:init', function () {
  Alpine.data('tags', function () {
    return {
      filter: '',

      init: function () {
        // No initial parsing required — filter starts empty and tag rows
        // render server-side. Method-based delete flow replaces the previous
        // window 'bb-delete-tag' custom event listener.
      },

      deleteTag: function (id, slug, count) {
        var n = parseInt(count, 10) || 0;
        var msg = 'Delete tag "' + slug + '"?';
        if (n > 0) {
          msg += '\n\nThis will remove the tag from ' + n + ' transaction' + (n === 1 ? '' : 's') + '. Activity annotations are preserved.';
        }
        bbConfirm({title: 'Delete tag?', message: msg, confirmLabel: 'Delete', variant: 'danger'}).then(function (ok) {
          if (!ok) return;
          fetch('/-/tags/' + id, {method: 'DELETE'})
            .then(function (r) {
              if (r.ok) {
                window.dispatchEvent(new CustomEvent('bb-toast', {detail: {message: 'Tag deleted', type: 'success'}}));
                setTimeout(function () { location.reload(); }, 400);
              } else {
                r.json().then(function (d) {
                  window.dispatchEvent(new CustomEvent('bb-toast', {detail: {message: (d.error && d.error.message) || 'Delete failed', type: 'error'}}));
                });
              }
            });
        });
      }
    };
  });
});
