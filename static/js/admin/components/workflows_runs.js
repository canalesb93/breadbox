// Workflows runs tab factory. Provides the per-row "Re-run workflow"
// action, which POSTs to the existing admin run-now endpoint (a workflow
// IS an agent_definition) and reloads so the fresh in-progress run shows
// at the top. CSRF token comes from the root element's data-csrf.
document.addEventListener('alpine:init', function () {
  Alpine.data('workflowsRuns', function () {
    return {
      csrfToken: '',

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

      // Re-run the workflow now. Fires a fresh run of the underlying
      // definition (no prompt override); on success reload to surface it.
      retrigger: function (slug) {
        var self = this;
        fetch('/-/workflows/' + encodeURIComponent(slug) + '/run', {
          method: 'POST',
          credentials: 'same-origin',
          headers: { 'X-CSRF-Token': this.csrfToken },
        })
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            window.location.reload();
          })
          .catch(function (e) {
            console.error('retrigger failed', e);
            self.restorePageState();
          });
      },
    };
  });
});
