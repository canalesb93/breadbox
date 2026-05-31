// Workflows gallery page factory. Enables presets (instantiate a workflow) and
// toggles an instantiated workflow's run state. CSRF token is read from the
// root element's data-csrf attribute. Registers via Alpine.data per the admin
// page convention (see docs/design-system.md → "Alpine page components").
document.addEventListener('alpine:init', function () {
  Alpine.data('workflowsGallery', function () {
    return {
      csrfToken: '',
      // open holds the slug of the preset whose configure drawer is showing
      // (empty string = no drawer).
      open: '',

      init: function () {
        this.csrfToken = this.$el.dataset.csrf || '';
      },

      restorePageState: function () {
        if (window.bbProgress) window.bbProgress.finish();
        var main = document.querySelector('main');
        if (main) {
          main.style.opacity = '';
          main.style.filter = '';
          main.style.pointerEvents = '';
        }
      },

      _post: function (url, body) {
        return fetch(url, {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'X-CSRF-Token': this.csrfToken,
          },
          body: body || '',
        });
      },

      // Submit the configure drawer: instantiate the workflow with the chosen
      // schedule / instructions / run-now, then reload so the row swaps its
      // "Set up" button for a run toggle.
      submitDrawer: function (slug, form) {
        var self = this;
        var fd = new FormData(form);
        // An unchecked checkbox is omitted by FormData — make the intent explicit.
        var box = form.querySelector('input[name="enabled"]');
        if (box && !box.checked) fd.set('enabled', 'false');
        var btn = form.querySelector('button[type="submit"]');
        if (btn) btn.disabled = true;
        self._post('/-/workflow-presets/' + encodeURIComponent(slug) + '/enable', new URLSearchParams(fd).toString())
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            window.location.reload();
          })
          .catch(function (e) {
            console.error('submitDrawer failed', e);
            if (btn) btn.disabled = false;
            self.restorePageState();
          });
      },

      // Toggle an instantiated workflow's run state. Reuses the agent
      // enable/disable endpoints (a workflow IS an agent_definition).
      toggleWorkflow: function (workflowSlug, el) {
        var self = this;
        var enabled = el.checked;
        var url = '/-/agents/' + encodeURIComponent(workflowSlug) + (enabled ? '/enable' : '/disable');
        self._post(url)
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
          })
          .catch(function (e) {
            console.error('toggleWorkflow failed', e);
            el.checked = !enabled; // revert
            self.restorePageState();
          });
      },
    };
  });
});
