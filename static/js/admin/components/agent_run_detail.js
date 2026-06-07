// Run detail Alpine component for /workflows/runs/{shortId}
// (legacy alias: /agents/runs/{shortId}).
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// Responsibilities:
//   - Live transcript polling for in_progress runs. Replaces the legacy
//     `<meta http-equiv="refresh">` that lost scroll position and any
//     open <details> nodes on every reload.
//   - Inline-edit operator note (PATCH /api/v1/workflows/runs/{id}, JSON
//     body). The session cookie + Origin check on the API side is what
//     authorises the request — no separate CSRF token needed for the
//     PATCH because /api/v1/* uses the same-host check.
//   - Duration ticker so the "started 4s ago" label keeps updating even
//     when no transcript event has landed yet.
//   - Copy run-id to clipboard.
//   - "Expand all / Collapse all" tool-call toggle across the thread.
//
// Three Alpine factories are registered:
//   - agentRunDetail — the page root component.
//   - agentRunNote   — the operator note card. Lives next to the root
//                       because note state is independent of polling.
//   - bbJsonViewer   — instance-local helper for the Copy button on
//                       individual JSON viewers. Lives here because the
//                       run-detail page is its only consumer for now;
//                       promote to a shared file if a second page picks
//                       up components.JSONViewer.

document.addEventListener('alpine:init', function () {
  Alpine.data('agentRunDetail', function () {
    return {
      // Wired from data-* attributes on the root element.
      shortId: '',
      status: '',
      startedAtMs: 0,
      pollSeconds: 0,
      pollTimer: null,
      allOpen: false,
      copiedRunId: false,
      // Re-run + copy-prompt affordances in the sticky header.
      workflowSlug: '',
      csrfToken: '',
      rerunning: false,
      cancelling: false,
      copiedPrompt: false,

      init: function () {
        var root = this.$el;
        this.shortId = root.dataset.runShortId || '';
        this.status = root.dataset.runStatus || '';
        this.workflowSlug = root.dataset.workflowSlug || '';
        this.csrfToken = root.dataset.csrf || '';
        this.pollSeconds = parseInt(root.dataset.pollSeconds || '0', 10);
        var startedAt = root.dataset.startedAt || '';
        if (startedAt) {
          var parsed = Date.parse(startedAt);
          if (!isNaN(parsed)) this.startedAtMs = parsed;
        }

        // Duration ticker runs whenever the run is in_progress. We
        // update the rendered duration cell in the sticky header.
        if (this.status === 'in_progress' && this.startedAtMs > 0) {
          this.tickDuration();
          var self = this;
          this._tickInterval = setInterval(function () { self.tickDuration(); }, 1000);
        }

        // Live polling: only when the server told us this run is still
        // in_progress AND it gave us a non-zero cadence.
        if (this.pollSeconds > 0 && this.status === 'in_progress') {
          var self2 = this;
          this.pollTimer = setTimeout(function () { self2.poll(); }, this.pollSeconds * 1000);
          // For long in_progress runs the operator usually wants to
          // see what just happened — anchor the view at the bottom
          // of the thread on first paint.
          this.$nextTick(function () { self2.scrollThreadToBottom(false); });
        }
      },

      // scrollThreadToBottom snaps the chat thread to its last event.
      // We only run this for in_progress runs — a finished run should
      // open at the top so the operator reads the conversation in
      // order. `smooth` toggles the scroll-behaviour: false for the
      // initial anchor (the operator hasn't moved yet), true after a
      // live-poll patch (so they notice new content land).
      scrollThreadToBottom: function (smooth) {
        var thread = this.$refs.thread;
        if (!thread) return;
        // We may be scrolling either the inner thread element (when it
        // has its own overflow) or the window (when the page itself
        // grows). Try the inner element first.
        try {
          thread.scrollIntoView({ block: 'end', behavior: smooth ? 'smooth' : 'auto' });
        } catch (e) {
          // older browsers — fall back to a manual offset
          window.scrollTo(0, document.body.scrollHeight);
        }
      },

      destroy: function () {
        if (this.pollTimer) clearTimeout(this.pollTimer);
        if (this._tickInterval) clearInterval(this._tickInterval);
      },

      tickDuration: function () {
        if (!this.startedAtMs) return;
        var elapsed = Date.now() - this.startedAtMs;
        var el = this.$refs.duration;
        if (el) el.textContent = formatDurationMs(elapsed);
      },

      poll: function () {
        var self = this;
        fetch('/-/workflows/runs/' + encodeURIComponent(this.shortId) + '/live', {
          headers: { 'Accept': 'application/json' },
          credentials: 'same-origin',
        })
          .then(function (r) {
            if (!r.ok) throw new Error('live poll failed: ' + r.status);
            return r.json();
          })
          .then(function (body) {
            self.applyLive(body);
            // Re-arm only if the run is still in_progress.
            if (self.status === 'in_progress') {
              self.pollTimer = setTimeout(function () { self.poll(); }, self.pollSeconds * 1000);
            } else if (self._tickInterval) {
              clearInterval(self._tickInterval);
              self._tickInterval = null;
            }
          })
          .catch(function (err) {
            // Soft-fail — keep polling so transient errors recover.
            console.warn('[agent run live]', err);
            if (self.status === 'in_progress') {
              self.pollTimer = setTimeout(function () { self.poll(); }, self.pollSeconds * 1000 * 2);
            }
          });
      },

      // applyLive patches the page with the server's latest snapshot.
      // The server returns the rendered transcript fragment + a few
      // scalar fields so we don't need to re-fetch the whole page or
      // round-trip individual events.
      //
      // Streaming polish:
      //   - Skip the DOM patch if the transcript HTML is byte-identical
      //     to the previous render. Avoids collapsing every <details>
      //     and re-initing lucide on a no-op tick.
      //   - Anchor scroll position by whether the user was near the
      //     bottom before the patch. If they were reading further up,
      //     we don't yank the page down on every new event.
      applyLive: function (body) {
        if (!body) return;
        var prevEventCount = -1;
        if (this.$refs.eventCount) {
          var m = (this.$refs.eventCount.textContent || '').match(/(\d+)/);
          if (m) prevEventCount = parseInt(m[1], 10);
        }
        if (typeof body.transcriptHTML === 'string') {
          var threadEl = this.$refs.thread;
          // Skip when nothing changed — saves the full innerHTML swap
          // (which would collapse every <details> and re-run lucide).
          if (threadEl && body.transcriptHTML !== this._lastTranscriptHTML) {
            this._lastTranscriptHTML = body.transcriptHTML;
            // Remember whether the operator was anchored at the bottom
            // BEFORE we patch — sticky-bottom is the conventional
            // chat-app affordance, but if they're reading from above we
            // shouldn't yank them.
            var wasAtBottom = nearViewportBottom();
            threadEl.innerHTML = body.transcriptHTML;
            // Re-init lucide icons for any newly-injected <i data-lucide>.
            if (window.lucide && typeof window.lucide.createIcons === 'function') {
              window.lucide.createIcons();
            }
            // The transcript fragment is server-rendered (markdown included
            // via goldmark), so there's no client-side markdown pass here.
            // Only auto-scroll when (a) more events landed AND (b) the
            // operator was already near the bottom. That keeps "reading
            // from the middle while a run streams" usable.
            if (
              this.status === 'in_progress' &&
              typeof body.eventCount === 'number' &&
              prevEventCount >= 0 &&
              body.eventCount > prevEventCount &&
              wasAtBottom
            ) {
              var self = this;
              this.$nextTick(function () { self.scrollThreadToBottom(true); });
            }
          }
        }
        if (typeof body.eventCount === 'number') {
          var ec = this.$refs.eventCount;
          if (ec) {
            ec.textContent = body.eventCount === 0 ? '' : body.eventCount + ' events';
          }
        }
        if (body.statsHTML && this.$refs.stats) {
          this.$refs.stats.innerHTML = body.statsHTML;
        }
        if (typeof body.status === 'string' && body.status !== this.status) {
          this.status = body.status;
          // The summary STATUS card is the single status indicator; swap its
          // markup in place so "running" lasts the run, then flips to terminal.
          if (this.$refs.summaryStatus) this.$refs.summaryStatus.innerHTML = body.summaryStatusHTML || '';
          if (window.lucide && typeof window.lucide.createIcons === 'function') {
            window.lucide.createIcons();
          }
          // Fire a toast on terminal transition so the operator notices
          // even if they scrolled away.
          if (body.status === 'success' || body.status === 'error' || body.status === 'cancelled') {
            var doneMsg = 'Run completed.';
            var doneType = 'success';
            if (body.status === 'error') { doneMsg = 'Run failed.'; doneType = 'error'; }
            else if (body.status === 'cancelled') { doneMsg = 'Run cancelled.'; doneType = 'warning'; }
            window.dispatchEvent(new CustomEvent('bb-toast', {
              detail: { message: doneMsg, type: doneType },
            }));
          }
        }
        if (typeof body.durationMs === 'number' && this.$refs.duration) {
          this.$refs.duration.textContent = formatDurationMs(body.durationMs);
        }
      },

      copyRunId: function () {
        var self = this;
        copyText(this.shortId).then(function (ok) {
          if (!ok) return;
          self.copiedRunId = true;
          setTimeout(function () { self.copiedRunId = false; }, 1400);
        });
      },

      toggleAll: function () {
        // Inside an Alpine @click handler, `this.$el` resolves to the
        // element the directive is on (the button) — NOT the component
        // root. Use `this.$root` to scope the query to the run-detail
        // x-data root. Capture the new state in a local up front so
        // the forEach callback isn't tangled in proxy/`this` quirks.
        var shouldOpen = !this.allOpen;
        this.allOpen = shouldOpen;
        var root = this.$root || document;
        root.querySelectorAll('.bb-tool, .bb-prompt-collapse').forEach(function (d) {
          if (shouldOpen) d.setAttribute('open', '');
          else d.removeAttribute('open');
        });
      },

      // restorePageState clears the global SPA progress bar + content
      // fade after an async action fails. Required on every error path
      // per .claude/rules/ui.md — without it the page stays blurred.
      restorePageState: function () {
        if (window.bbProgress) window.bbProgress.finish();
        var main = document.querySelector('main');
        if (main) {
          main.style.opacity = '';
          main.style.filter = '';
          main.style.pointerEvents = '';
        }
      },

      // copyPrompt copies the prompt this run was given (per-run prefix +
      // user prompt, server-assembled into the hidden <template x-ref>).
      // Mirrors copyRunId's copied-flag flip for the inline confirmation.
      copyPrompt: function () {
        var tpl = this.$refs.promptText;
        if (!tpl) return;
        // <template> content lives in .content; fall back to textContent
        // for browsers that flatten it (none we target, but cheap).
        var text = (tpl.content ? tpl.content.textContent : tpl.textContent) || '';
        text = text.trim();
        if (!text) return;
        var self = this;
        copyText(text).then(function (ok) {
          if (!ok) return;
          self.copiedPrompt = true;
          window.dispatchEvent(new CustomEvent('bb-toast', {
            detail: { message: 'Prompt copied', type: 'success' },
          }));
          setTimeout(function () { self.copiedPrompt = false; }, 1600);
        });
      },

      // rerun fires a fresh run of this run's workflow via the canonical
      // admin run-now endpoint (a workflow IS an agent_definition) and,
      // on success, navigates to the new run's detail page so the
      // operator watches it stream. No prompt override — this re-runs the
      // workflow as configured. Concurrency / budget guards surface as a
      // toast rather than a silent failure.
      rerun: function () {
        if (this.rerunning || !this.workflowSlug) return;
        this.rerunning = true;
        var self = this;
        fetch('/-/workflows/' + encodeURIComponent(this.workflowSlug) + '/run', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Accept': 'application/json',
            'X-CSRF-Token': this.csrfToken,
          },
        })
          .then(function (res) {
            return res.json().then(function (body) { return { ok: res.ok, body: body }; });
          })
          .then(function (r) {
            if (!r.ok) {
              var msg = (r.body && r.body.error && r.body.error.message) || 'Could not start the run.';
              throw new Error(msg);
            }
            window.dispatchEvent(new CustomEvent('bb-toast', {
              detail: { message: 'Run started', type: 'success' },
            }));
            var newId = r.body && r.body.short_id;
            if (newId) {
              window.location.href = '/workflows/runs/' + encodeURIComponent(newId);
            } else {
              window.location.reload();
            }
          })
          .catch(function (err) {
            self.rerunning = false;
            self.restorePageState();
            window.dispatchEvent(new CustomEvent('bb-toast', {
              detail: { message: err.message || 'Could not start the run.', type: 'error' },
            }));
            console.warn('[agent run rerun]', err);
          });
      },

      // cancelRun aborts an in-progress run. Confirms first (work done so far is
      // kept, the rest is abandoned), then POSTs to the cancel endpoint. The
      // server SIGKILLs the sidecar and lands the row as 'cancelled'; the live
      // poll picks that up and the button hides itself (x-show on status).
      cancelRun: function () {
        if (this.cancelling || this.status !== 'in_progress') return;
        var self = this;
        var go = function () { self._doCancel(); };
        if (typeof window.bbConfirm !== 'function') {
          if (window.confirm('Stop this run now? Work done so far is kept; the rest is abandoned.')) go();
          return;
        }
        window.bbConfirm({
          title: 'Cancel this run?',
          message: 'Stop the run now. Anything it has already written is kept; the remaining work is abandoned. You can re-run it afterwards.',
          confirmLabel: 'Cancel run',
          variant: 'danger',
        }).then(function (ok) { if (ok) go(); });
      },

      _doCancel: function () {
        if (this.cancelling) return;
        this.cancelling = true;
        var self = this;
        fetch('/-/workflows/runs/' + encodeURIComponent(this.shortId) + '/cancel', {
          method: 'POST',
          credentials: 'same-origin',
          headers: { 'Accept': 'application/json', 'X-CSRF-Token': this.csrfToken },
        })
          .then(function (res) {
            return res.json().catch(function () { return null; }).then(function (body) {
              return { ok: res.ok || res.status === 202, body: body };
            });
          })
          .then(function (r) {
            if (!r.ok) {
              var msg = (r.body && r.body.error && r.body.error.message) || 'Could not cancel the run.';
              throw new Error(msg);
            }
            window.dispatchEvent(new CustomEvent('bb-toast', {
              detail: { message: 'Cancelling run…', type: 'info' },
            }));
            // Nudge a poll soon so the terminal status lands promptly instead of
            // waiting a full cadence; the poller hides the button once it flips.
            if (self.pollTimer) clearTimeout(self.pollTimer);
            self.pollTimer = setTimeout(function () { self.poll(); }, 1200);
          })
          .catch(function (err) {
            self.cancelling = false;
            self.restorePageState();
            window.dispatchEvent(new CustomEvent('bb-toast', {
              detail: { message: err.message || 'Could not cancel the run.', type: 'error' },
            }));
            console.warn('[agent run cancel]', err);
          });
      },
    };
  });

  Alpine.data('agentRunNote', function () {
    return {
      shortId: '',
      value: '',
      draft: '',
      editing: false,
      saving: false,
      error: '',

      init: function () {
        var root = this.$el;
        this.shortId = root.dataset.shortId || '';
        this.value = (root.dataset.initial || '').toString();
      },

      startEdit: function () {
        this.draft = this.value || '';
        this.editing = true;
        this.error = '';
        var self = this;
        this.$nextTick(function () {
          if (self.$refs.textarea) {
            self.$refs.textarea.focus();
            self.$refs.textarea.setSelectionRange(self.draft.length, self.draft.length);
          }
        });
      },

      cancel: function () {
        this.editing = false;
        this.draft = '';
        this.error = '';
      },

      save: function () {
        if (this.saving) return;
        this.saving = true;
        this.error = '';
        var self = this;
        var trimmed = (this.draft || '').slice(0, 2000);
        fetch('/api/v1/workflows/runs/' + encodeURIComponent(this.shortId), {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify({ note: trimmed }),
        })
          .then(function (r) {
            if (!r.ok) throw new Error('save failed: ' + r.status);
            return r.json();
          })
          .then(function () {
            self.value = trimmed;
            self.editing = false;
            self.draft = '';
            window.dispatchEvent(new CustomEvent('bb-toast', {
              detail: { message: 'Note saved', type: 'success' },
            }));
          })
          .catch(function (err) {
            self.error = 'Could not save — try again.';
            console.warn('[agent run note save]', err);
          })
          .finally(function () { self.saving = false; });
      },
    };
  });

  Alpine.data('bbJsonViewer', function () {
    return {
      copied: false,
      copyJson: function (scriptEl) {
        if (!scriptEl) return;
        var text = (scriptEl.textContent || '').trim();
        var self = this;
        copyText(text).then(function (ok) {
          if (!ok) return;
          self.copied = true;
          setTimeout(function () { self.copied = false; }, 1200);
        });
      },
    };
  });
});

// ── helpers ───────────────────────────────────────────────────────────

function copyText(s) {
  // Prefer the async clipboard API; fall back to execCommand for older
  // browsers / non-secure contexts so the affordance still works.
  if (navigator.clipboard && navigator.clipboard.writeText) {
    return navigator.clipboard.writeText(s).then(function () { return true; }, function () { return false; });
  }
  return new Promise(function (resolve) {
    var ta = document.createElement('textarea');
    ta.value = s;
    ta.style.position = 'fixed';
    ta.style.top = '-10000px';
    document.body.appendChild(ta);
    ta.select();
    var ok = false;
    try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
    document.body.removeChild(ta);
    resolve(ok);
  });
}

// nearViewportBottom returns true when the viewport is within ~120px
// of the bottom of the document — the threshold we use to decide
// whether to keep an in_progress run anchored at the latest event.
// 120px is roughly two chat-bubble heights, generous enough that a
// reader at the end of the visible transcript is still considered
// "anchored" even if they haven't scrolled the very last pixel.
function nearViewportBottom() {
  var scrollY = window.scrollY || window.pageYOffset || 0;
  var viewportH = window.innerHeight || document.documentElement.clientHeight;
  var docH = Math.max(
    document.body.scrollHeight,
    document.documentElement.scrollHeight,
    document.body.offsetHeight,
    document.documentElement.offsetHeight
  );
  return docH - (scrollY + viewportH) < 120;
}

function formatDurationMs(ms) {
  if (ms < 1000) return ms + 'ms';
  var s = ms / 1000;
  if (s < 60) return s.toFixed(1) + 's';
  var m = s / 60;
  if (m < 60) return m.toFixed(1) + 'm';
  var h = m / 60;
  return h.toFixed(1) + 'h';
}
