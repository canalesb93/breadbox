// Agent Reports Alpine component for /dashboard.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Powers the dashAgentReportsCard list — tracks which reports are dismissed
// (mark-as-read) so they fade out without a page reload, and posts the read
// state back to the server.
document.addEventListener('alpine:init', function () {
  Alpine.data('agentReports', function () {
    return {
      dismissed: {},

      init: function () {
        // No initial state to parse — dismissed map starts empty and fills
        // as the user marks reports as read.
      },

      markRead: function (id) {
        this.dismissed[id] = true;
        fetch('/-/reports/' + id + '/read', { method: 'POST' });
      },

      markAllRead: function () {
        var self = this;
        document.querySelectorAll('[id^="report-"]').forEach(function (el) {
          var id = el.id.replace('report-', '');
          self.dismissed[id] = true;
        });
        fetch('/-/reports/read-all', { method: 'POST' });
      },
    };
  });
});
