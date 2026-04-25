// Backups page scripts for /backups.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// This page does not use a named Alpine.data() factory — the root <div>
// uses an empty x-data plus x-init/x-destroy to set the keyboard-shortcut
// scope. Two pieces of behavior live here:
//
//   1. If the URL contains a hash (#schedule or #restore), scroll the
//      matching collapsible card into view after a short delay so the
//      Alpine x-show transition has time to expand it.
//   2. Register a backups-scoped keyboard shortcut (`n`) that clicks the
//      `[data-new-backup]` button so the real POST form handles CSRF +
//      preflight gating.
//
// The <script src> loads synchronously at the top of the templ component
// (so the alpine:init listener registers before Alpine fires the event).
// That means this script runs BEFORE the rest of the body is parsed, so
// the hash-scroll IIFE has to wait for DOMContentLoaded to query the
// target element.
function bbBackupsScrollToHash() {
  if (!location.hash) return;
  var el = document.querySelector(location.hash);
  if (el) {
    setTimeout(function () {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }, 100);
  }
}
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bbBackupsScrollToHash);
} else {
  bbBackupsScrollToHash();
}

// Register backups-scoped keyboard shortcuts. `n` clicks the Create Backup
// submit button so the real POST form handles CSRF + preflight gating —
// the shortcut is purely a UI affordance.
document.addEventListener('alpine:init', function () {
  var reg = Alpine.store('shortcuts');
  if (!reg) return;

  reg.register({
    id: 'backups.new',
    keys: 'n',
    description: 'Create backup',
    group: 'New',
    scope: 'backups',
    action: function () {
      var btn = document.querySelector('[data-new-backup]');
      if (btn && !btn.disabled) btn.click();
    },
  });
});
