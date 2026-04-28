// Feed page Alpine factories for /feed.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
//   - `feedSyncNow` — backs the "Sync now" button on the empty-state card.
//     Triggers a household-wide sync (POST /-/connections/sync-all) and
//     reloads the page once the kickoff returns 2xx so the freshly written
//     sync_logs surface in the feed timeline. Admin-only; the templ guards
//     the button with `if p.IsAdmin`, so this factory is only ever bound
//     under an admin session.

document.addEventListener('alpine:init', function () {
  Alpine.data('feedSyncNow', function () {
    return {
      state: 'idle',

      triggerSyncNow: function () {
        if (this.state !== 'idle') return;
        this.state = 'syncing';
        var self = this;
        fetch('/-/connections/sync-all', { method: 'POST' })
          .then(function (res) {
            if (res.ok) {
              self.state = 'done';
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Sync triggered. Reloading…', type: 'success' } }));
              setTimeout(function () { window.location.reload(); }, 1200);
            } else {
              self.state = 'idle';
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Failed to trigger sync.', type: 'error' } }));
            }
          })
          .catch(function () {
            self.state = 'idle';
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Network error. Please try again.', type: 'error' } }));
          });
      },
    };
  });
});
