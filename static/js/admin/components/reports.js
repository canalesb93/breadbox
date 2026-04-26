// Reports inbox Alpine component for /reports.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
// No server-side data hand-off needed — dismissed state is purely client-side.
document.addEventListener('alpine:init', function () {
  Alpine.data('reportsPage', function () {
    return {
      dismissed: {},

      init: function () {},

      markRead: function (id) {
        this.dismissed[id] = true;
        fetch('/-/reports/' + id + '/read', { method: 'POST' });
      }
    };
  });
});
