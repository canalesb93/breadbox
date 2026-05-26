// Run-an-agent modal Alpine component.
//
// The agentRunModal templ in internal/templates/components/pages/agents_shell.templ
// renders one <dialog x-data="agentRunModal"> per page (mounted by both
// /agents and /agents/definitions). The payload — list of enabled agents +
// per-agent last-prompt-prefix — is handed in via a sibling
// @templ.JSONScript with id "<modalID>-data".
//
// Pattern reference: docs/design-system.md → "Alpine page components".
document.addEventListener('alpine:init', function () {
  Alpine.data('agentRunModal', function () {
    return {
      agents: [],
      lastPrefixes: {},
      query: '',
      prefix: '',
      busy: false,
      csrf: '',

      init: function () {
        // Discover the payload script via the modal's id; the script
        // tag is sibling-rendered as "<modalID>-data".
        var root = this.$root;
        this.csrf = root.getAttribute('data-csrf') || '';
        var dataEl = document.getElementById(root.id + '-data');
        if (dataEl) {
          try {
            var payload = JSON.parse(dataEl.textContent) || {};
            this.agents = Array.isArray(payload.agents) ? payload.agents : [];
            this.lastPrefixes = payload.last_prefixes || {};
          } catch (e) {
            console.error('agentRunModal: failed to parse payload', e);
          }
        }
      },

      get filtered() {
        var q = (this.query || '').trim().toLowerCase();
        if (!q) return this.agents;
        return this.agents.filter(function (a) {
          return (
            (a.Name || '').toLowerCase().indexOf(q) !== -1 ||
            (a.Description || '').toLowerCase().indexOf(q) !== -1 ||
            (a.Slug || '').toLowerCase().indexOf(q) !== -1
          );
        });
      },

      lastPrefixFor: function (slug) {
        return this.lastPrefixes[slug] || '';
      },

      toast: function (message, type) {
        window.dispatchEvent(
          new CustomEvent('bb-toast', { detail: { message: message, type: type || 'info' } })
        );
      },

      restorePageState: function () {
        if (window.bbProgress) {
          try { window.bbProgress.finish(); } catch (e) {}
        }
        var main = document.querySelector('main');
        if (main) {
          main.style.opacity = '';
          main.style.filter = '';
          main.style.pointerEvents = '';
        }
      },

      run: function (slug) {
        if (this.busy) return;
        this.busy = true;
        var prefix = (this.prefix || '').trim();
        var body = '';
        if (prefix) {
          body = 'prompt_prefix=' + encodeURIComponent(prefix);
        }
        var self = this;
        fetch('/-/agents/' + encodeURIComponent(slug) + '/run', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'X-CSRF-Token': self.csrf,
            'Content-Type': 'application/x-www-form-urlencoded',
          },
          body: body,
        })
          .then(function (res) {
            self.busy = false;
            if (!res.ok) {
              self.toast(
                res.status === 503
                  ? 'Another run is already in progress.'
                  : 'Failed to start agent.',
                'error'
              );
              self.restorePageState();
              return;
            }
            self.toast('Agent run started.', 'success');
            self.$root.close();
            setTimeout(function () { window.location.reload(); }, 700);
          })
          .catch(function () {
            self.busy = false;
            self.toast('Network error. Please try again.', 'error');
            self.restorePageState();
          });
      },
    };
  });
});
