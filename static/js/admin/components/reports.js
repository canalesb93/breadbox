// Reports inbox Alpine component for /reports.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
// No server-side data hand-off needed — dismissed state is purely client-side.
// CSRF tokens are injected into these POSTs by base.html's global fetch
// interceptor, so no token plumbing is needed here.
document.addEventListener('alpine:init', function () {
  Alpine.data('reportsPage', function () {
    return {
      dismissed: {},

      init: function () {},

      flash: function (message, type) {
        window.dispatchEvent(new CustomEvent('bb-toast', {
          detail: { message: message, type: type || 'error' }
        }));
      },

      // Optimistically collapse the row, then POST. On failure, re-show the
      // row and surface a toast so a dropped request isn't silently lost
      // (the row would otherwise stay dismissed until a full reload).
      markRead: async function (id) {
        this.dismissed[id] = true;
        try {
          var res = await fetch('/-/reports/' + id + '/read', { method: 'POST' });
          if (!res.ok) throw new Error(String(res.status));
        } catch (e) {
          delete this.dismissed[id];
          this.flash('Could not mark read — try again');
        }
      },

      // Mark every report read, then reload to reflect the cleared inbox.
      // Reload only on success; a failed request toasts instead of reloading
      // into an apparently-unchanged list.
      markAllRead: async function () {
        try {
          var res = await fetch('/-/reports/read-all', { method: 'POST' });
          if (!res.ok) throw new Error(String(res.status));
          window.location.reload();
        } catch (e) {
          this.flash('Could not mark all read — try again');
        }
      }
    };
  });
});
