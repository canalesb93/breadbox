package pages

// backupsInlineScript returns the inline <script> block lifted out of the
// old backups.html template body. The script does two things:
//
//   1. If the URL contains a hash (#schedule or #restore) the matching
//      collapsible card scrolls into view after a short delay so the
//      Alpine x-show transition has time to expand it.
//   2. Registers a keyboard shortcut (`n`) under the `backups` scope set
//      by the page's outer x-init. The shortcut clicks the
//      `[data-new-backup]` button so the real POST form handles CSRF +
//      preflight gating.
//
// The script has no template inputs, so it is rendered with @templ.Raw.
func backupsInlineScript() string {
	return `<script>
(function() {
  if (!location.hash) return;
  var el = document.querySelector(location.hash);
  if (el) setTimeout(function() { el.scrollIntoView({ behavior: 'smooth', block: 'start' }); }, 100);
})();

// Register backups-scoped keyboard shortcuts. ` + "`n`" + ` clicks the Create Backup
// submit button so the real POST form handles CSRF + preflight gating —
// the shortcut is purely a UI affordance.
document.addEventListener('alpine:init', function() {
  var reg = Alpine.store('shortcuts');
  if (!reg) return;

  reg.register({
    id: 'backups.new',
    keys: 'n',
    description: 'Create backup',
    group: 'New',
    scope: 'backups',
    action: function() {
      var btn = document.querySelector('[data-new-backup]');
      if (btn && !btn.disabled) btn.click();
    },
  });
});
</script>`
}
