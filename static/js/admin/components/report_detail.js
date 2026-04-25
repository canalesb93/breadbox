// Report-detail Alpine component for /reports/{id}.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Initial scalar state (report ID + is-read flag) flows in via data-*
// attributes on the x-data root and is read in init().
//
// The DOMContentLoaded listener at the bottom runs DOMPurify.sanitize(
// marked.parse(body)) over each `.bb-report-body` element and rewrites
// external links to open in a new tab. It depends on the marked and
// dompurify CDN scripts loaded as siblings of this file in the templ.
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

// External-link targeting only — styling lives in .bb-report-body (input.css).
document.addEventListener('DOMContentLoaded', function () {
  document.querySelectorAll('.bb-report-body').forEach(function (el) {
    var md = el.getAttribute('data-markdown');
    if (!md) return;
    el.innerHTML = DOMPurify.sanitize(marked.parse(md));
    el.querySelectorAll('a').forEach(function (a) {
      var href = a.getAttribute('href') || '';
      if (a.href && !a.href.startsWith(window.location.origin) && !href.startsWith('/')) {
        a.setAttribute('target', '_blank');
        a.setAttribute('rel', 'noopener');
      }
    });
    el.querySelectorAll('table').forEach(function (t) {
      if (t.parentNode && t.parentNode.classList && t.parentNode.classList.contains('bb-report-table-wrap')) return;
      var wrap = document.createElement('div');
      wrap.className = 'bb-report-table-wrap';
      t.parentNode.insertBefore(wrap, t);
      wrap.appendChild(t);
    });
    var lastChild = el.lastElementChild;
    if (lastChild) lastChild.classList.add('!mb-0');
  });
});
