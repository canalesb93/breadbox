// Users page scripts for /settings/household.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// This page does not use a named Alpine.data() factory — the root <div>
// uses an empty x-data plus x-init/x-destroy to set the keyboard-shortcut
// scope. One shortcut is registered here:
//
//   `n` — clicks the Add Member link so the browser handles normal navigation.
//
// The <script src> loads synchronously at the top of the templ component
// (so the alpine:init listener registers before Alpine fires the event).

// Register users-scoped keyboard shortcuts. `n` clicks the Add Member
// link (anchor tag). Using .click() respects the browser's normal
// navigation so we don't need to duplicate the href here.
document.addEventListener('alpine:init', function () {
  var reg = Alpine.store('shortcuts');
  if (!reg) return;

  reg.register({
    id: 'users.new',
    keys: 'n',
    description: 'Add member',
    group: 'New',
    scope: 'users',
    action: function () {
      var el = document.querySelector('[data-new-user]');
      if (el) el.click();
    },
  });
});
