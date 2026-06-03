// Developer Mode reporter — the always-on-top bug/task filer rendered (gated
// by .DevModeEnabled) at the end of base.html. Submit POSTs the report to
// /-/dev-reports, which returns a prefilled GitHub issue-draft URL; the client
// opens it for the user to review and submit. No token, no screenshot/HTML
// hosting yet (those return once a remote storage backend is wired in). CSRF is
// added by the global fetch wrapper in base.html. See internal/admin/dev_reports.go.

(function () {
  document.addEventListener('alpine:init', function () {
    Alpine.data('devReporter', function () {
      return {
        panelOpen: false,
        type: 'bug',
        title: '',
        description: '',
        submitting: false,
        error: '',
        pagePath: location.pathname + location.search,
        page: '',
        user: '',
        version: '',

        init: function () {
          var ds = this.$root.dataset || {};
          this.page = ds.page || '';
          this.user = ds.user || '';
          this.version = ds.version || '';

          var reg = Alpine.store('shortcuts');
          if (reg && typeof reg.register === 'function') {
            var self = this;
            reg.register({
              id: 'dev.report',
              keys: 'g b',
              description: 'Report a bug or task',
              group: 'Developer',
              scope: 'global',
              action: function () { if (!self.panelOpen) self.open(); },
            });
          }
        },

        open: function () {
          this.error = '';
          this.pagePath = location.pathname + location.search;
          this.panelOpen = true;
          var self = this;
          this.$nextTick(function () {
            if (self.$refs.titleInput) self.$refs.titleInput.focus();
          });
        },

        close: function () { this.panelOpen = false; },

        reset: function () {
          this.type = 'bug';
          this.title = '';
          this.description = '';
          this.error = '';
        },

        metadata: function () {
          var theme = document.documentElement.getAttribute('data-theme');
          if (!theme) {
            theme = (window.matchMedia && matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
          }
          return {
            viewport: window.innerWidth + '×' + window.innerHeight,
            theme: theme,
            user_agent: navigator.userAgent,
            app_version: this.version,
            current_page: this.page,
            reported_by: this.user,
            client_time: new Date().toISOString(),
          };
        },

        submit: function () {
          if (!this.title.trim() || this.submitting) return;
          this.submitting = true;
          this.error = '';
          var self = this;
          var payload = {
            type: this.type,
            title: this.title.trim(),
            description: this.description,
            // Drop the query string + hash — they can carry search terms.
            page_url: location.origin + location.pathname,
            page_path: location.pathname,
            metadata: this.metadata(),
          };
          fetch('/-/dev-reports', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
          }).then(function (resp) {
            return resp.json().catch(function () { return {}; }).then(function (data) {
              return { ok: resp.ok, data: data };
            });
          }).then(function (r) {
            self.submitting = false;
            if (!r.ok) {
              self.error = (r.data && r.data.error && r.data.error.message) || 'Failed to prepare the report.';
              return;
            }
            var data = r.data || {};
            if (data.draft_url) {
              // Open the prefilled GitHub draft. window.open can be blocked
              // after an async fetch, so always surface a toast link too.
              try { window.open(data.draft_url, '_blank', 'noopener'); } catch (e) {}
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: {
                message: 'GitHub draft ready — review & submit',
                type: 'info',
                href: data.draft_url,
                linkLabel: 'Open draft',
                duration: 8000,
              } }));
            } else {
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: {
                message: 'Report prepared',
                type: 'success',
              } }));
            }
            self.panelOpen = false;
            self.reset();
          }).catch(function () {
            self.submitting = false;
            self.error = 'Network error — please try again.';
          });
        },
      };
    });
  });
})();
