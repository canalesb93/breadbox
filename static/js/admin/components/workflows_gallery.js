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
      // previewCopied briefly flips true after a successful Copy so the
      // button can show a "Copied" confirmation.
      previewCopied: false,
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
        // avatarSeed drives the header DiceBear preview + posts as the hidden
        // avatar_seed field. Empty = seed on the slug (the historical default).
        avatarSeed: '',
        enabled: false, // run-state, driven by the header toggle (toggleWorkflow)
        // triggerOnSync is a STRING ('true' | 'false') so the trigger radios
        // can bind via x-model and submit as the trigger_on_sync form field.
        triggerOnSync: 'false',
        scheduleCron: '',
        model: '',
        maxTurns: '',
        maxBudget: '',
        additionalInstructions: '',
        options: [], // [{ key, label, help, selected, choices: [{value,label}] }]
      },
      // -------------------------------------------------------------------

      // --- Schedule preview ----------------------------------------------
      // cronPreview holds the human-readable rendering of the current cron
      // (from GET /-/workflows/cron-preview). Shared by both drawers since
      // only one is open at a time. describeCron() refreshes it.
      cronPreview: '',
      cronPreviewLoading: false,
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
      openReconfigure: function (slug, name, enabled) {
        var self = this;
        self.reconfigure.slug = slug;
        self.reconfigure.name = name || slug;
        self.reconfigure.enabled = !!enabled;
        self.reconfigure.loading = true;
        self.reconfigure.options = [];
        self.reconfigure.additionalInstructions = '';
        self.reconfigure.scheduleCron = '';
        self.reconfigure.triggerOnSync = 'false';
        self.reconfigure.model = '';
        self.reconfigure.maxTurns = '';
        self.reconfigure.maxBudget = '';
        self.reconfigure.avatarSeed = '';
        self.cronPreview = '';
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
            self.reconfigure.triggerOnSync = data.trigger_on_sync ? 'true' : 'false';
            self.reconfigure.scheduleCron = data.schedule_cron || '';
            self.reconfigure.model = data.model || '';
            self.reconfigure.maxTurns = data.max_turns ? String(data.max_turns) : '';
            self.reconfigure.maxBudget = data.max_budget_usd ? String(data.max_budget_usd) : '';
            self.reconfigure.additionalInstructions = data.additional_instructions || '';
            self.reconfigure.avatarSeed = data.avatar_seed || '';
            self.reconfigure.options = Array.isArray(data.options) ? data.options : [];
            if (self.reconfigure.triggerOnSync === 'false') self.describeCron(self.reconfigure.scheduleCron);
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

      // reconfigureAvatarSrc builds the header avatar URL for the workflow's
      // current seed, falling back to the slug when no custom seed is set. The
      // ?type=agent param picks the agent DiceBear style server-side; the server
      // also maps a slug to its stored avatar_seed, so the live preview (raw
      // seed) and the saved render (slug-keyed) resolve to the same image.
      reconfigureAvatarSrc: function () {
        var seed = this.reconfigure.avatarSeed || this.reconfigure.slug || 'workflow';
        return '/avatars/' + encodeURIComponent(seed) + '?type=agent&size=88';
      },

      // shuffleAvatar mints a fresh random seed so the operator can cycle to a
      // different DiceBear mark. The preview updates reactively; the seed posts
      // with the form (hidden avatar_seed) on Save.
      shuffleAvatar: function () {
        var bytes = new Uint8Array(8);
        if (window.crypto && window.crypto.getRandomValues) {
          window.crypto.getRandomValues(bytes);
        } else {
          for (var i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
        }
        var hex = '';
        for (var j = 0; j < bytes.length; j++) hex += ('0' + bytes[j].toString(16)).slice(-2);
        this.reconfigure.avatarSeed = hex;
      },

      // describeCron fetches a human-readable rendering of a cron expression
      // for the schedule preview. Debounced at the call site
      // (@input.debounce in the template); here it just fetches + stores.
      describeCron: function (cron) {
        var self = this;
        cron = (cron || '').trim();
        if (!cron) {
          self.cronPreview = '';
          return;
        }
        self.cronPreviewLoading = true;
        // Pass the viewer's IANA timezone so the preview renders the schedule
        // in their local time (the scheduler fires cron in the server's tz).
        var tz = '';
        try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch (e) { tz = ''; }
        fetch('/-/workflows/cron-preview?cron=' + encodeURIComponent(cron) + (tz ? '&tz=' + encodeURIComponent(tz) : ''), {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) {
            return res.json();
          })
          .then(function (data) {
            self.cronPreview = data && data.description ? data.description : '';
          })
          .catch(function () {
            self.cronPreview = '';
          })
          .finally(function () {
            self.cronPreviewLoading = false;
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
          // The body element is reused across opens; reset its scroll so a
          // new prompt always starts at the top rather than wherever the
          // previous one was left scrolled. The element is x-show-gated on
          // previewLoading, so a same-tick reset can land while it's still
          // display:none (a no-op). rAF defers until after x-show flushes and
          // the element is laid out, so the reset actually sticks.
          el.scrollTop = 0;
          requestAnimationFrame(function () { el.scrollTop = 0; });
        });
      },

      // copyPrompt copies the raw (markdown source) workflow prompt to the
      // clipboard. Bound to the modal's Copy button and the 'c' shortcut.
      // Flashes previewCopied for confirmation. Falls back to a hidden
      // textarea + execCommand on browsers without the async clipboard API.
      copyPrompt: function () {
        var self = this;
        var text = self.previewBody || '';
        if (!text) return;
        var done = function () {
          self.previewCopied = true;
          setTimeout(function () { self.previewCopied = false; }, 1500);
        };
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(done).catch(function () {
            self._copyFallback(text, done);
          });
        } else {
          self._copyFallback(text, done);
        }
      },

      _copyFallback: function (text, done) {
        try {
          var ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.opacity = '0';
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
          done();
        } catch (e) {
          console.error('copyPrompt fallback failed', e);
        }
      },
      // -------------------------------------------------------------------
    };
  });
});
