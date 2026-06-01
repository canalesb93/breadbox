// Workflows gallery page factory. Enables presets (instantiate a workflow) and
// toggles an instantiated workflow's run state. CSRF token is read from the
// root element's data-csrf attribute. Registers via Alpine.data per the admin
// page convention (see docs/design-system.md → "Alpine page components").
document.addEventListener('alpine:init', function () {
  Alpine.data('workflowsGallery', function () {
    return {
      csrfToken: '',
      // Drawer open/close is owned by the global $store.drawers store
      // (see layout/base.html). The configure drawers are keyed
      // 'wf-config-<presetSlug>'; the shared reconfigure drawer is
      // 'wf-reconfigure'. open()/close() are called from the template
      // (Set up / Cancel / Escape / backdrop) and from openReconfigure().

      // --- F2: preview internal prompt -----------------------------------
      // State for the "Preview prompt" modal: the composed base prompt is
      // fetched on demand from /-/workflows/{slug}/prompt and rendered as
      // markdown (via bbRenderMarkdown) into the x-ref="previewBody" element.
      // previewLoading drives the in-modal spinner.
      previewTitle: '',
      previewBody: '',
      previewLoading: false,
      // -------------------------------------------------------------------

      // --- F3: reconfigure an enabled workflow ---------------------------
      // The single, data-driven reconfigure drawer is opened by re-using the
      // same `open` slot with the special value 'reconfigure'. Its fields are
      // hydrated from GET /-/workflows/{slug}/config so the drawer renders
      // prefilled with the workflow's live schedule, options, and additional
      // instructions. reconfigure.loading drives the in-drawer spinner.
      reconfigure: {
        loading: false,
        slug: '',
        name: '',
        triggerOnSync: false,
        scheduleCron: '',
        additionalInstructions: '',
        options: [], // [{ key, label, help, selected, choices: [{value,label}] }]
      },
      // -------------------------------------------------------------------

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

      // projectedCost returns the drawer's reactive cost hint for a scheduled
      // preset: estPerRun × the chosen cadence's runs/month. Cadences map to a
      // rough monthly run count (daily≈30, weekly≈4.3, monthly=1). Approximate
      // by design — it's an order-of-magnitude transparency cue, not a quote.
      projectedCost: function (cron, estPerRun, postSync) {
        var est = Number(estPerRun) || 0;
        if (postSync) return '≈ $' + est.toFixed(2) + ' per run';
        var perMonth = { '0 8 * * *': 30, '0 7 * * 1': 4.3, '0 8 1 * *': 1 }[cron] || 4.3;
        var runs = Math.round(perMonth);
        return '≈ $' + (est * perMonth).toFixed(2) + '/mo · ' + runs + (runs === 1 ? ' run' : ' runs');
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

      // Toggle an instantiated workflow's run state via the Workflows
      // enable/disable endpoints (a workflow IS an agent_definition).
      toggleWorkflow: function (workflowSlug, el) {
        var self = this;
        var enabled = el.checked;
        var url = '/-/workflows/' + encodeURIComponent(workflowSlug) + (enabled ? '/enable' : '/disable');
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

      // --- F1: run an enabled workflow now -------------------------------
      // toast dispatches the global bb-toast event (handled in base.html).
      toast: function (message, type) {
        window.dispatchEvent(
          new CustomEvent('bb-toast', { detail: { message: message, type: type || 'info' } })
        );
      },

      // Run an enabled workflow on demand. The run is async server-side
      // (202 Accepted returns the in_progress row immediately), so we show an
      // optimistic "started" toast the instant the request is accepted rather
      // than waiting on the run. Concurrency/budget/auth refusals come back as
      // error envelopes — surface their message instead of the optimistic toast.
      runWorkflow: function (workflowSlug) {
        var self = this;
        self.toast('Starting run…', 'info');
        self._post('/-/workflows/' + encodeURIComponent(workflowSlug) + '/run')
          .then(function (res) {
            if (res.ok) {
              self.toast('Workflow run started.', 'success');
              return;
            }
            return res
              .json()
              .catch(function () {
                return null;
              })
              .then(function (body) {
                var msg = (body && body.error && body.error.message) || 'Could not start the run.';
                self.toast(msg, 'error');
              });
          })
          .catch(function (e) {
            console.error('runWorkflow failed', e);
            self.toast('Network error — could not start the run.', 'error');
            self.restorePageState();
          });
      },
      // -------------------------------------------------------------------

      // --- F3: reconfigure an enabled workflow ---------------------------
      // Open the shared reconfigure drawer prefilled with an enabled
      // workflow's live config (schedule, options, additional instructions),
      // fetched from GET /-/workflows/{slug}/config.
      openReconfigure: function (slug, name) {
        var self = this;
        self.reconfigure.slug = slug;
        self.reconfigure.name = name || slug;
        self.reconfigure.loading = true;
        self.reconfigure.options = [];
        self.reconfigure.additionalInstructions = '';
        self.reconfigure.scheduleCron = '';
        self.reconfigure.triggerOnSync = false;
        Alpine.store('drawers').open('wf-reconfigure');
        fetch('/-/workflows/' + encodeURIComponent(slug) + '/config', {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
          })
          .then(function (data) {
            self.reconfigure.name = data.name || self.reconfigure.name;
            self.reconfigure.triggerOnSync = !!data.trigger_on_sync;
            self.reconfigure.scheduleCron = data.schedule_cron || '';
            self.reconfigure.additionalInstructions = data.additional_instructions || '';
            self.reconfigure.options = Array.isArray(data.options) ? data.options : [];
          })
          .catch(function (e) {
            console.error('openReconfigure failed', e);
            Alpine.store('drawers').close();
            self.restorePageState();
          })
          .finally(function () {
            self.reconfigure.loading = false;
          });
      },

      // Submit the reconfigure drawer: re-compose the workflow's schedule,
      // options, and additional instructions via POST
      // /-/workflows/{slug}/reconfigure, then reload to reflect the new
      // trigger label. The run-state toggle is untouched by a reconfigure.
      submitReconfigure: function (form) {
        var self = this;
        var slug = self.reconfigure.slug;
        if (!slug) return;
        var fd = new FormData(form);
        var btn = form.querySelector('button[type="submit"]');
        if (btn) btn.disabled = true;
        self._post('/-/workflows/' + encodeURIComponent(slug) + '/reconfigure', new URLSearchParams(fd).toString())
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            window.location.reload();
          })
          .catch(function (e) {
            console.error('submitReconfigure failed', e);
            if (btn) btn.disabled = false;
            self.restorePageState();
          });
      },
      // -------------------------------------------------------------------

      // --- F2: preview internal prompt -----------------------------------
      // Open the shared "Preview prompt" dialog and fetch the preset's fully
      // composed base prompt (read-only). The <dialog id="wf-prompt-preview">
      // lives once in the gallery template; this opens it and fills the body.
      previewPrompt: function (slug, name) {
        var self = this;
        self.previewTitle = name || slug;
        self.previewBody = '';
        self.previewLoading = true;
        var dialog = document.getElementById('wf-prompt-preview');
        if (dialog && typeof dialog.showModal === 'function') dialog.showModal();
        fetch('/-/workflows/' + encodeURIComponent(slug) + '/prompt', {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
          })
          .then(function (data) {
            self.previewTitle = data.title || self.previewTitle;
            self.previewBody = data.prompt || '';
          })
          .catch(function (e) {
            console.error('previewPrompt failed', e);
            self.previewBody = 'Could not load the prompt for this workflow. Please try again.';
          })
          .finally(function () {
            self.previewLoading = false;
            self.renderPreviewBody();
          });
      },

      // Render the fetched base prompt as markdown into the preview element.
      // The body arrives async and the modal is reused across opens, so we
      // clear bbRenderMarkdown's idempotency flag (data-markdown-rendered)
      // before re-pointing data-markdown at the new content. Falls back to
      // plain text if the shared renderer isn't loaded.
      renderPreviewBody: function () {
        var self = this;
        self.$nextTick(function () {
          var el = self.$refs.previewBody;
          if (!el) return;
          el.removeAttribute('data-markdown-rendered');
          el.setAttribute('data-markdown', self.previewBody || '');
          if (typeof window.bbRenderMarkdown === 'function') {
            window.bbRenderMarkdown(el);
          } else {
            el.textContent = self.previewBody || '';
          }
        });
      },
      // -------------------------------------------------------------------
    };
  });
});
