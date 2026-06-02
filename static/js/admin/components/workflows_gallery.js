// Workflows gallery page factory. Enables presets (instantiate a workflow) and
// toggles an instantiated workflow's run state. CSRF token is read from the
// root element's data-csrf attribute. Registers via Alpine.data per the admin
// page convention (see docs/design-system.md → "Alpine page components").
document.addEventListener('alpine:init', function () {
  // ---- cron timezone shift ------------------------------------------------
  //
  // The scheduler fires cron in the SERVER's local timezone, but the schedule
  // shortcut pills express a friendly hour in the VIEWER's timezone ("Daily" =
  // 9 AM your time). The helpers below convert a viewer-local cron into the
  // server-local cron we store + submit — the exact inverse of the shift the
  // cron-preview endpoint (service.DescribeCronInTZ) applies to render a stored
  // cron back in the viewer's time, so the two round-trip. Ported from
  // service.shiftCronTimeFields so the client and server agree.

  function cronSingleInt(s) {
    return /^\d+$/.test(s) ? parseInt(s, 10) : null;
  }

  // Shift a day-of-week field (single value or comma list, 0/7 = Sunday) by
  // dayDelta days, wrapping within the week. Returns null for ranges, steps, or
  // named days so the caller falls back to the unshifted expression.
  function cronShiftDow(field, dayDelta) {
    var parts = field.split(',');
    var out = [];
    for (var i = 0; i < parts.length; i++) {
      var p = parts[i].trim();
      if (!/^\d+$/.test(p)) return null;
      var n = parseInt(p, 10);
      if (n < 0 || n > 7) return null;
      if (n === 7) n = 0; // normalize Sunday
      out.push(String(((n + dayDelta) % 7 + 7) % 7));
    }
    return out.join(',');
  }

  // Shift a standard 5-field cron's time-of-day by deltaMin minutes, carrying a
  // midnight wrap into the day-of-week set. Returns null for the
  // non-representable cases (non-integer minute/hour, or a day-of-month
  // constrained schedule whose wrap would land on a different calendar day) so
  // the caller keeps the original. Mirrors service.shiftCronTimeFields.
  function cronShiftTimeFields(expr, deltaMin) {
    var f = String(expr).trim().split(/\s+/);
    if (f.length !== 5) return null;
    var minute = cronSingleInt(f[0]);
    var hour = cronSingleInt(f[1]);
    if (minute === null || hour === null) return null;
    var total = hour * 60 + minute + deltaMin;
    var dayDelta = 0;
    while (total < 0) { total += 1440; dayDelta--; }
    while (total >= 1440) { total -= 1440; dayDelta++; }
    f[0] = String(total % 60);
    f[1] = String(Math.floor(total / 60));
    if (dayDelta !== 0) {
      if (f[2] !== '*' || f[3] !== '*') return null; // monthly/dom + wrap → too risky
      if (f[4] !== '*') { // "*" is daily — a wrap leaves it daily
        var shifted = cronShiftDow(f[4], dayDelta);
        if (shifted === null) return null;
        f[4] = shifted;
      }
    }
    return f.join(' ');
  }
  // -------------------------------------------------------------------------

  Alpine.data('workflowsGallery', function () {
    return {
      csrfToken: '',
      // Server's UTC offset in minutes (east of UTC), seeded from the root
      // element's data-server-utc-offset-min. Drives localCronToServer() so the
      // schedule shortcut pills resolve to the viewer's local hour. 0 (no
      // shift) is the right fallback for the common self-hosted case where the
      // server and the viewer share a timezone.
      serverUtcOffsetMin: 0,
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
        var off = parseInt(this.$el.dataset.serverUtcOffsetMin || '0', 10);
        this.serverUtcOffsetMin = isNaN(off) ? 0 : off;
      },

      // localCronToServer converts a viewer-local cron (what a shortcut pill
      // means — e.g. "0 9 * * *" is 9 AM in the viewer's timezone) into the
      // server-local cron the scheduler stores + fires. Falls back to the input
      // unchanged when there's no timezone delta (server tz == viewer tz, the
      // common self-hosted case) or the shift isn't representable.
      localCronToServer: function (localExpr) {
        var viewerOff;
        try {
          viewerOff = -new Date().getTimezoneOffset(); // minutes east of UTC
        } catch (e) {
          return localExpr;
        }
        var deltaMin = this.serverUtcOffsetMin - viewerOff; // viewer-local → server-local
        if (!deltaMin) return localExpr;
        return cronShiftTimeFields(localExpr, deltaMin) || localExpr;
      },

      // cronPillActive highlights a shortcut pill when the current
      // (server-local) cron equals that pill's viewer-local intent converted to
      // server time — so a pill stays lit whether it was clicked, typed, or
      // hydrated from a saved workflow, and across timezones.
      cronPillActive: function (localExpr, currentCron) {
        return this.localCronToServer(localExpr).trim() === String(currentCron || '').trim();
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
