// Report-detail Alpine component for /reports/{id}.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Initial scalar state (report ID + is-read flag) flows in via data-*
// attributes on the x-data root and is read in init().
//
// The report body renders server-side via @components.MarkdownLarge
// (goldmark) into .bb-prose — no client-side markdown. This file is purely
// the Alpine factory for the toolbar (mark-read, copy-link, toast).
document.addEventListener('alpine:init', function () {
  Alpine.data('reportDetail', function () {
    return {
      id: '',
      isRead: false,
      toast: '',
      _toastTimer: null,

      init: function () {
        this.id = this.$el.dataset.reportId || '';
        this.isRead = this.$el.dataset.isRead === 'true';
      },

      flash: function (msg) {
        this.toast = msg;
        clearTimeout(this._toastTimer);
        var self = this;
        this._toastTimer = setTimeout(function () { self.toast = ''; }, 1800);
      },

      toggleRead: async function () {
        var endpoint = this.isRead ? 'unread' : 'read';
        try {
          var res = await fetch('/-/reports/' + this.id + '/' + endpoint, { method: 'POST' });
          if (!res.ok) throw new Error(String(res.status));
          this.isRead = !this.isRead;
          this.flash(this.isRead ? 'Marked as read' : 'Marked as unread');
          // Re-render the icon for the toggled state.
          this.$nextTick(function () {
            if (window.lucide && window.lucide.createIcons) window.lucide.createIcons();
          });
        } catch (e) {
          this.flash('Could not update — try again');
        }
      },

      copyLink: function () {
        try {
          navigator.clipboard.writeText(window.location.href);
          this.flash('Link copied');
        } catch (e) {
          this.flash('Copy failed');
        }
      }
    };
  });
});

