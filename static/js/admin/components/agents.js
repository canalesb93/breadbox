// Outer tab switcher for /agents — guide / wizard / settings / activity.
// Initial tab read from data-initial-tab on the x-data root. The watcher
// keeps `?tab=...` in sync via history.replaceState so deep links and
// the back button still work without a full reload.
//
// Convention reference: docs/design-system.md → "Alpine page components".
document.addEventListener('alpine:init', function () {
  Alpine.data('agentsTabs', function () {
    return {
      tab: 'guide',
      init: function () {
        var t = this.$el.dataset.initialTab;
        if (t) this.tab = t;
        this.$watch('tab', function (v) {
          var u = new URL(window.location);
          u.searchParams.set('tab', v);
          history.replaceState(null, '', u);
        });
      },
    };
  });
});
