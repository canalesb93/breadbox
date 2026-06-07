// Workflows gallery page factory. Enables presets (instantiate a workflow) and
// toggles an instantiated workflow's run state. CSRF token is read from the
// root element's data-csrf attribute. Registers via Alpine.data per the admin
// page convention (see docs/design-system.md → "Alpine page components").
document.addEventListener('alpine:init', function () {
  // The schedule trigger's cron input — chips, custom field, live preview, and
  // the viewer-local → server-local timezone shift — lives in the shared
  // cronField component (static/js/admin/components/cron_field.js). This factory
  // just owns the drawers around it; the cron value two-way binds via the
  // CronField's ModelExpr (the setup drawer's `cron`, reconfigure's
  // `reconfigure.scheduleCron`).

  Alpine.data('workflowsGallery', function () {
    return {
      csrfToken: '',
      // Drawer open/close is owned by the global $store.drawers store
      // (see layout/base.html). The configure drawers are keyed
      // 'wf-config-<presetSlug>'; the shared reconfigure drawer is
      // 'wf-reconfigure'. open()/close() are called from the template
      // (Set up / Cancel / Escape / backdrop) and from openReconfigure().

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
        // oneOff hides the trigger picker + run-state toggle for on-demand
        // workflows (they only ever run via Run now). Hydrated from /config.
        oneOff: false,
        // triggerOnSync is a STRING ('true' | 'false') so the trigger radios
        // can bind via x-model and submit as the trigger_on_sync form field.
        triggerOnSync: 'false',
        scheduleCron: '',
        model: '',
        maxTurns: '',
        maxBudget: '',
        additionalInstructions: '',
        options: [], // [{ key, label, help, selected, choices: [{value,label}] }]
        connectors: [], // [{ name, url, enabled }] — library connectors + per-workflow enablement
      },
      // -------------------------------------------------------------------

      // --- Custom (hand-authored) workflow drawer ------------------------
      // One drawer for both create (openCustom('')) and edit
      // (openCustom(slug), hydrated from GET /-/custom-workflows/{slug}).
      // The operator authors the whole prompt; there's no preset template.
      // triggerOnSync is a STRING ('true' | 'false') so the trigger radios
      // bind via x-model; the CronField two-way binds custom.scheduleCron.
      custom: {
        loading: false,
        isEdit: false,
        slug: '',
        name: '',
        prompt: '',
        // avatarSeed drives the header EditableAvatar preview + posts as the
        // hidden avatar_seed field. Empty = slug-seeded (the default).
        avatarSeed: '',
        // triggerMode is the 3-way trigger: 'manual' | 'schedule' | 'sync'.
        // Posts as trigger_mode; the schedule CronField (custom.scheduleCron)
        // only applies when 'schedule'.
        triggerMode: 'manual',
        scheduleCron: '',
        model: 'claude-sonnet-4-6',
        toolScope: 'read_write',
        maxTurns: '',
        maxBudget: '',
        enabled: true, // create-only "Activate" toggle
      },
      // -------------------------------------------------------------------

      // runningOneOff[slug] is true while a one-off's Run-now request is in
      // flight, so each card's inline Run button can show a spinner + disable
      // itself (preventing a double-dispatch). Keyed by preset slug since two
      // one-off cards can be on screen at once.
      runningOneOff: {},
      // -------------------------------------------------------------------

      // pendingSetupSlug tracks a not-yet-set-up card whose run toggle was
      // optimistically flipped on while its setup drawer is open. If that
      // drawer is dismissed without saving (Cancel / Esc / backdrop), the
      // drawer-close watcher in init() reverts the toggle. A successful save
      // reloads the page, so the optimistic state is replaced by server truth.
      pendingSetupSlug: '',
      // -------------------------------------------------------------------

      init: function () {
        this.csrfToken = this.$el.dataset.csrf || '';
        var self = this;
        // Revert an optimistic setup toggle when its drawer closes unsaved.
        // submitDrawer reloads on success (no close call), so the only way the
        // config drawer leaves the screen with a pending toggle is a dismiss.
        this.$watch('$store.drawers.active', function (active) {
          if (
            self.pendingSetupSlug &&
            active !== 'wf-config-' + self.pendingSetupSlug
          ) {
            self._revertPendingSetup();
          }
        });
      },

      // cardClick makes the whole preset row act as its run toggle. A click
      // that lands on an actual control (the toggle, the settings gear, a link)
      // is left to that control; anywhere else forwards to the row's toggle via
      // a synthesized click, so the toggle's own @change / @click handler runs
      // (toggleWorkflow for set-up cards, beginSetup for not-yet-set-up ones).
      // Wired only on recurring (non one-off) cards for admins.
      cardClick: function (ev) {
        if (ev.target.closest('button, a, input, label, [role="button"]')) return;
        var toggle = ev.currentTarget.querySelector('input.toggle:not([disabled])');
        if (toggle) toggle.click();
      },

      // beginSetup handles a not-yet-set-up card's toggle: optimistically show
      // it enabled and open the setup drawer. Tracked by pendingSetupSlug so a
      // dismissed drawer reverts it (see the init() watcher).
      beginSetup: function (slug, el) {
        if (el) el.checked = true;
        this.pendingSetupSlug = slug;
        Alpine.store('drawers').open('wf-config-' + slug);
      },

      _revertPendingSetup: function () {
        var slug = this.pendingSetupSlug;
        this.pendingSetupSlug = '';
        if (!slug) return;
        var cb = document.querySelector('input.toggle[data-setup-slug="' + slug + '"]');
        if (cb) cb.checked = false;
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
      // toast dispatches the global bb-toast event (handled in base.html). opts
      // may carry { href, linkLabel, duration } so a run toast can deep-link to
      // the run it just started and linger long enough to click.
      toast: function (message, type, opts) {
        opts = opts || {};
        window.dispatchEvent(
          new CustomEvent('bb-toast', {
            detail: {
              message: message,
              type: type || 'info',
              href: opts.href || '',
              linkLabel: opts.linkLabel || 'View',
              duration: opts.duration || 0,
            },
          })
        );
      },

      // runStartedToast turns a 202 run row into a success toast that deep-links
      // to the run detail page (/workflows/runs/{short_id}). Falls back to a
      // plain toast when the row has no short_id.
      runStartedToast: function (run) {
        var sid = run && run.short_id;
        if (sid) {
          this.toast('Run started.', 'success', {
            href: '/workflows/runs/' + encodeURIComponent(sid),
            linkLabel: 'View run',
            duration: 6000,
          });
        } else {
          this.toast('Run started.', 'success', { duration: 4000 });
        }
      },

      // runErrorToast maps a run error envelope to a toast, attaching a helpful
      // action link where one exists (settings for auth/budget, the runs list
      // when another run holds the lock). Returns false for CONSENT_REQUIRED so
      // the caller can route to the setup drawer instead of toasting.
      runErrorToast: function (body) {
        var code = body && body.error && body.error.code;
        var msg = (body && body.error && body.error.message) || 'Could not start the run.';
        if (code === 'CONSENT_REQUIRED') return false;
        if (code === 'CONCURRENCY_LOCKED') {
          this.toast('A run is already in progress.', 'warning', {
            href: '/workflows/runs', linkLabel: 'View runs', duration: 6000,
          });
          return true;
        }
        if (code === 'AUTH_NOT_CONFIGURED' || code === 'BINARY_NOT_FOUND' || code === 'AGENTS_DISABLED') {
          this.toast(msg, 'error', { href: '/settings/workflows', linkLabel: 'Configure', duration: 7000 });
          return true;
        }
        if (code === 'BUDGET_CEILING_REACHED') {
          this.toast(msg, 'error', { href: '/settings/workflows', linkLabel: 'Adjust', duration: 7000 });
          return true;
        }
        this.toast(msg, 'error', { duration: 5000 });
        return true;
      },

      // Run an enabled workflow on demand (the reconfigure drawer's Run now).
      // The run is async server-side (202 returns the in_progress row), so the
      // success toast deep-links to that run rather than waiting on completion.
      runWorkflow: function (workflowSlug) {
        var self = this;
        self._post('/-/workflows/' + encodeURIComponent(workflowSlug) + '/run')
          .then(function (res) {
            if (res.ok || res.status === 202) {
              return res.json().catch(function () { return null; }).then(function (run) {
                self.runStartedToast(run);
              });
            }
            return res.json().catch(function () { return null; }).then(function (body) {
              self.runErrorToast(body);
            });
          })
          .catch(function (e) {
            console.error('runWorkflow failed', e);
            self.toast('Network error — could not start the run.', 'error');
            self.restorePageState();
          });
      },
      // -------------------------------------------------------------------

      // --- One-off (on-demand) workflows ---------------------------------
      // Run an on-demand workflow from its card's Run button. The endpoint
      // instantiates the manual-only workflow on first use, then dispatches the
      // run, so this one call covers first-run and re-run alike. CONSENT_REQUIRED
      // (first-ever workflow) routes to the setup drawer instead of erroring. No
      // full-page reload — the deep-link is the post-run affordance.
      //
      // The run is async server-side (the 202 returns the in_progress row in an
      // instant, then a goroutine does the actual work). So we hold the spinner
      // up (runningOneOff[slug]) through the whole run by polling the run's
      // status until it reaches a terminal state — not just for the dispatch.
      runOneOff: function (slug, name) {
        var self = this;
        if (self.runningOneOff[slug]) return; // guard against a double-click
        name = name || slug;
        var go = function () { self._dispatchOneOff(slug); };
        // Confirm before spending: a one-off runs Claude over the household's
        // ledger and bills the Anthropic account, so gate the click behind the
        // shared confirm overlay (amber/cost tone, not destructive-red).
        if (typeof window.bbConfirm !== 'function') {
          if (window.confirm('Run "' + name + '" now? This runs Claude over your financial data and incurs API cost.')) go();
          return;
        }
        window.bbConfirm({
          title: 'Run workflow now?',
          message: 'Run "' + name + '" now? It runs Claude over your household’s financial data and incurs Anthropic API cost.',
          confirmLabel: 'Run now',
          variant: 'warning',
        }).then(function (ok) { if (ok) go(); });
      },

      // _dispatchOneOff performs the actual run dispatch (instantiate-on-first-use
      // + async run + spinner poll). Split out of runOneOff so the confirm gate
      // wraps it without duplicating the dispatch logic.
      _dispatchOneOff: function (slug) {
        var self = this;
        if (self.runningOneOff[slug]) return; // guard against a double-click
        self.runningOneOff[slug] = true;
        self._post('/-/workflow-presets/' + encodeURIComponent(slug) + '/run')
          .then(function (res) {
            if (res.ok || res.status === 202) {
              return res.json().catch(function () { return null; }).then(function (run) {
                self.runStartedToast(run);
                if (run && run.short_id) {
                  // Keep the spinner up and follow the run to completion.
                  self._pollOneOffRun(slug, run.short_id);
                } else {
                  self.runningOneOff[slug] = false; // can't track it — release the button
                }
              });
            }
            return res.json().catch(function () { return null; }).then(function (body) {
              self.runningOneOff[slug] = false;
              if (!self.runErrorToast(body)) {
                // CONSENT_REQUIRED — send them through the setup drawer, which
                // carries the consent checkbox, rather than a dead-end toast.
                self.toast('Confirm setup to run this workflow.', 'info');
                Alpine.store('drawers').open('wf-config-' + slug);
              }
            });
          })
          .catch(function (e) {
            console.error('runOneOff failed', e);
            self.runningOneOff[slug] = false;
            self.toast('Network error — could not start the run.', 'error');
            self.restorePageState();
          });
      },

      // _pollOneOffRun polls a run's status (short_id + status) every few
      // seconds, holding runningOneOff[slug] true until the run reaches a
      // terminal state, then clears the spinner and toasts the outcome. Caps the
      // poll so a stuck run can't spin forever, and releases the button on a
      // poll error rather than leaving it disabled.
      _pollOneOffRun: function (slug, shortId) {
        var self = this;
        var attempts = 0;
        var maxAttempts = 200; // ~200 × 3s ≈ 10 min ceiling
        var tick = function () {
          fetch('/-/workflows/runs/' + encodeURIComponent(shortId) + '/status', {
            credentials: 'same-origin',
            headers: { Accept: 'application/json' },
          })
            .then(function (res) {
              if (!res.ok) throw new Error('HTTP ' + res.status);
              return res.json();
            })
            .then(function (data) {
              var status = data && data.status;
              if (status && status !== 'in_progress') {
                self.runningOneOff[slug] = false;
                self._oneOffDoneToast(status, shortId);
                return;
              }
              attempts++;
              if (attempts >= maxAttempts) {
                self.runningOneOff[slug] = false; // give up tracking; run may still finish
                return;
              }
              setTimeout(tick, 3000);
            })
            .catch(function () {
              // Can't confirm status — stop spinning so the button isn't stuck.
              self.runningOneOff[slug] = false;
            });
        };
        setTimeout(tick, 3000);
      },

      // _oneOffDoneToast announces a finished one-off run, deep-linking to it.
      _oneOffDoneToast: function (status, shortId) {
        var href = '/workflows/runs/' + encodeURIComponent(shortId);
        if (status === 'success') {
          this.toast('Run finished.', 'success', { href: href, linkLabel: 'View run', duration: 6000 });
        } else if (status === 'error') {
          this.toast('Run failed.', 'error', { href: href, linkLabel: 'View run', duration: 8000 });
        } else {
          this.toast('Run ' + status + '.', 'warning', { href: href, linkLabel: 'View run', duration: 6000 });
        }
      },

      // copyPromptDirect copies a workflow's composed base prompt straight to
      // the clipboard (no modal), for the card's copy-prompt icon. Fetches the
      // same /prompt endpoint the preview modal uses, then writes the markdown
      // source. Reuses the _copyFallback used by the modal's Copy button.
      copyPromptDirect: function (slug) {
        var self = this;
        fetch('/-/workflows/' + encodeURIComponent(slug) + '/prompt', {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
          })
          .then(function (data) {
            var text = (data && data.prompt) || '';
            if (!text) {
              self.toast('No prompt to copy.', 'error');
              return;
            }
            var done = function () { self.toast('Prompt copied to clipboard.', 'success'); };
            if (navigator.clipboard && navigator.clipboard.writeText) {
              navigator.clipboard.writeText(text).then(done).catch(function () {
                self._copyFallback(text, done);
              });
            } else {
              self._copyFallback(text, done);
            }
          })
          .catch(function (e) {
            console.error('copyPromptDirect failed', e);
            self.toast('Could not copy the prompt.', 'error');
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
        self.reconfigure.oneOff = false;
        self.reconfigure.options = [];
        self.reconfigure.additionalInstructions = '';
        self.reconfigure.scheduleCron = '';
        self.reconfigure.triggerOnSync = 'false';
        self.reconfigure.model = '';
        self.reconfigure.maxTurns = '';
        self.reconfigure.maxBudget = '';
        self.reconfigure.avatarSeed = '';
        self.reconfigure.connectors = [];
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
            self.reconfigure.oneOff = !!data.one_off;
            self.reconfigure.triggerOnSync = data.trigger_on_sync ? 'true' : 'false';
            self.reconfigure.scheduleCron = data.schedule_cron || '';
            self.reconfigure.model = data.model || '';
            self.reconfigure.maxTurns = data.max_turns ? String(data.max_turns) : '';
            self.reconfigure.maxBudget = data.max_budget_usd ? String(data.max_budget_usd) : '';
            self.reconfigure.additionalInstructions = data.additional_instructions || '';
            self.reconfigure.avatarSeed = data.avatar_seed || '';
            self.reconfigure.options = Array.isArray(data.options) ? data.options : [];
            self.reconfigure.connectors = (Array.isArray(data.connectors) ? data.connectors : []).map(function (c) {
              return { name: c.name, url: c.url || '', enabled: !!c.enabled };
            });
            // The embedded CronField is two-way bound to reconfigure.scheduleCron
            // (its ModelExpr), so the hydrated value pushes in and the component
            // refreshes its own live preview — no manual priming needed here.
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

      // Open the shared custom-workflow drawer. With no slug it opens blank
      // for a create; with a slug it hydrates from GET /-/custom-workflows/{slug}
      // for an edit. Resets to create defaults first so a prior edit's values
      // don't bleed in.
      openCustom: function (slug) {
        var self = this;
        self.custom.isEdit = !!slug;
        self.custom.slug = slug || '';
        self.custom.name = '';
        self.custom.prompt = '';
        self.custom.avatarSeed = '';
        self.custom.triggerMode = 'manual';
        self.custom.scheduleCron = '';
        self.custom.model = 'claude-sonnet-4-6';
        self.custom.toolScope = 'read_write';
        self.custom.maxTurns = '';
        self.custom.maxBudget = '';
        self.custom.enabled = true;
        self.custom.loading = !!slug;
        Alpine.store('drawers').open('wf-custom');
        if (!slug) return;
        fetch('/-/custom-workflows/' + encodeURIComponent(slug), {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
          })
          .then(function (data) {
            self.custom.name = data.name || '';
            self.custom.prompt = data.prompt || '';
            self.custom.scheduleCron = data.schedule_cron || '';
            // Derive the 3-way mode from the stored trigger_on_sync + cron.
            if (data.trigger_on_sync) {
              self.custom.triggerMode = 'sync';
            } else if (data.schedule_cron) {
              self.custom.triggerMode = 'schedule';
            } else {
              self.custom.triggerMode = 'manual';
            }
            self.custom.model = data.model || 'claude-sonnet-4-6';
            self.custom.toolScope = data.tool_scope || 'read_write';
            self.custom.maxTurns = data.max_turns ? String(data.max_turns) : '';
            self.custom.maxBudget = data.max_budget_usd ? String(data.max_budget_usd) : '';
            self.custom.avatarSeed = data.avatar_seed || '';
            self.custom.enabled = !!data.enabled;
          })
          .catch(function (e) {
            console.error('openCustom failed', e);
            Alpine.store('drawers').close();
            self.restorePageState();
          })
          .finally(function () {
            self.custom.loading = false;
          });
      },

      // Submit the custom-workflow drawer: POST to /-/custom-workflows (create)
      // or /-/custom-workflows/{slug} (edit), then reload so the card reflects
      // the new state.
      submitCustom: function (form) {
        var self = this;
        var fd = new FormData(form);
        // The create "Activate" checkbox is omitted by FormData when unchecked.
        var box = form.querySelector('input[name="enabled"]');
        if (box && !box.checked) fd.set('enabled', 'false');
        var url = self.custom.isEdit
          ? '/-/custom-workflows/' + encodeURIComponent(self.custom.slug)
          : '/-/custom-workflows';
        var btn = form.querySelector('button[type="submit"]');
        if (btn) btn.disabled = true;
        self._post(url, new URLSearchParams(fd).toString())
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            window.location.reload();
          })
          .catch(function (e) {
            console.error('submitCustom failed', e);
            if (btn) btn.disabled = false;
            self.restorePageState();
          });
      },

      // customAvatarSrc builds the header avatar URL for the custom drawer,
      // preferring the chosen seed and falling back to the slug (on edit) or a
      // generic seed (on create, before a slug exists). Mirrors
      // reconfigureAvatarSrc.
      customAvatarSrc: function () {
        var seed = this.custom.avatarSeed || this.custom.slug || 'workflow';
        return '/avatars/' + encodeURIComponent(seed) + '?type=agent&size=88';
      },

      // shuffleCustomAvatar mints a fresh random seed so the operator can cycle
      // to a different DiceBear mark; the preview updates reactively and the
      // seed posts as avatar_seed on save. Mirrors shuffleAvatar.
      shuffleCustomAvatar: function () {
        var bytes = new Uint8Array(8);
        if (window.crypto && window.crypto.getRandomValues) {
          window.crypto.getRandomValues(bytes);
        } else {
          for (var i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
        }
        var hex = '';
        for (var j = 0; j < bytes.length; j++) hex += ('0' + bytes[j].toString(16)).slice(-2);
        this.custom.avatarSeed = hex;
      },

      // copyCustomPrompt copies a custom workflow's prompt to the clipboard from
      // its config endpoint (the preset /prompt endpoint only knows presets).
      // Backs the copy icon on manual custom cards.
      copyCustomPrompt: function (slug) {
        var self = this;
        fetch('/-/custom-workflows/' + encodeURIComponent(slug), {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
          })
          .then(function (data) {
            var text = (data && data.prompt) || '';
            if (!text) {
              self.toast('No prompt to copy.', 'error');
              return;
            }
            var done = function () { self.toast('Prompt copied to clipboard.', 'success'); };
            if (navigator.clipboard && navigator.clipboard.writeText) {
              navigator.clipboard.writeText(text).then(done).catch(function () {
                self._copyFallback(text, done);
              });
            } else {
              self._copyFallback(text, done);
            }
          })
          .catch(function (e) {
            console.error('copyCustomPrompt failed', e);
            self.toast('Could not copy the prompt.', 'error');
          });
      },

      // runCustomNow runs a manual custom workflow from its card's Run button,
      // mirroring runOneOff's UX (cost-confirm gate + a spinner held for the
      // whole run via runningOneOff[slug]). Unlike a one-off preset, the
      // workflow already exists, so it dispatches /-/workflows/{slug}/run.
      runCustomNow: function (slug, name) {
        var self = this;
        if (self.runningOneOff[slug]) return; // guard against a double-click
        name = name || slug;
        var go = function () { self._dispatchCustomRun(slug); };
        if (typeof window.bbConfirm !== 'function') {
          if (window.confirm('Run "' + name + '" now? This runs Claude over your financial data and incurs API cost.')) go();
          return;
        }
        window.bbConfirm({
          title: 'Run workflow now?',
          message: 'Run "' + name + '" now? It runs Claude over your household’s financial data and incurs Anthropic API cost.',
          confirmLabel: 'Run now',
          variant: 'warning',
        }).then(function (ok) { if (ok) go(); });
      },

      _dispatchCustomRun: function (slug) {
        var self = this;
        if (self.runningOneOff[slug]) return;
        self.runningOneOff[slug] = true;
        self._post('/-/workflows/' + encodeURIComponent(slug) + '/run')
          .then(function (res) {
            if (res.ok || res.status === 202) {
              return res.json().catch(function () { return null; }).then(function (run) {
                self.runStartedToast(run);
                if (run && run.short_id) {
                  self._pollOneOffRun(slug, run.short_id);
                } else {
                  self.runningOneOff[slug] = false;
                }
              });
            }
            return res.json().catch(function () { return null; }).then(function (body) {
              self.runningOneOff[slug] = false;
              self.runErrorToast(body);
            });
          })
          .catch(function (e) {
            console.error('runCustomNow failed', e);
            self.runningOneOff[slug] = false;
            self.toast('Network error — could not start the run.', 'error');
            self.restorePageState();
          });
      },

      // Remove a configured workflow — deletes its agent_definition, resetting
      // the preset card back to "Set up". Confirms first via the shared
      // bbConfirm overlay (the schedule stops; run history survives the
      // SET NULL FK), then POSTs to /-/workflows/{slug}/delete and reloads.
      // Falls back to a native confirm only if bbConfirm hasn't loaded.
      removeWorkflow: function (slug, name) {
        var self = this;
        slug = slug || self.reconfigure.slug;
        if (!slug) return;
        name = name || self.reconfigure.name || 'this workflow';
        var doRemove = function () { self._doRemoveWorkflow(slug); };
        if (typeof window.bbConfirm !== 'function') {
          if (window.confirm('Remove "' + name + '"? This resets it back to a preset. Run history is kept.')) doRemove();
          return;
        }
        window.bbConfirm({
          title: 'Remove workflow?',
          message: 'Remove "' + name + '"? It resets back to a preset you can set up again. The schedule stops and run history is kept.',
          confirmLabel: 'Remove workflow',
          variant: 'danger',
        }).then(function (ok) { if (ok) doRemove(); });
      },

      _doRemoveWorkflow: function (slug) {
        var self = this;
        self._post('/-/workflows/' + encodeURIComponent(slug) + '/delete')
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            self.toast('Workflow removed.', 'success');
            setTimeout(function () { window.location.reload(); }, 500);
          })
          .catch(function (e) {
            console.error('removeWorkflow failed', e);
            self.toast('Could not remove the workflow.', 'error');
            self.restorePageState();
          });
      },
      // -------------------------------------------------------------------

      // --- F2: preview internal prompt -----------------------------------
      // Open the shared global prompt modal (#bb-prompt-modal, mounted once in
      // base.html) in read-only preview mode. We fetch the preset's composed
      // base prompt, then dispatch bb-prompt-modal with the raw markdown
      // (value, for the Copy action) plus the server-rendered, sanitized HTML
      // (html, so the modal skips its own /-/markdown/preview round-trip).
      // This is the single prompt-preview surface — no bespoke dialog here.
      previewPrompt: function (slug, name) {
        var self = this;
        fetch('/-/workflows/' + encodeURIComponent(slug) + '/prompt', {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) {
            if (!res.ok) throw new Error('HTTP ' + res.status);
            return res.json();
          })
          .then(function (data) {
            window.dispatchEvent(new CustomEvent('bb-prompt-modal', {
              detail: {
                mode: 'preview',
                title: data.title || name || slug,
                subtitle: "The built-in base prompt this workflow runs with, before any additional instructions.",
                value: data.prompt || '',
                html: data.prompt_html || '',
              },
            }));
          })
          .catch(function (e) {
            console.error('previewPrompt failed', e);
            self.toast('Could not load the prompt for this workflow.', 'error');
          });
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
