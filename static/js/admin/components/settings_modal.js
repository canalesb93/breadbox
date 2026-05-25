// Alpine factory for the global Settings modal mounted in base.html.
//
// Boot:
//   - If the dialog has data-initial-tab, the server is rendering a
//     deep-link landing — open the dialog immediately. When
//     data-initial-prefilled === "true" the body is already inlined; we
//     just open. Otherwise we fetch the tab body first.
//
// Runtime:
//   - `open-settings` window event opens the modal (used by the sidebar
//     gear and the cmdk palette). Detail: { tab: 'account' | ... }.
//   - `switchTo(tab)` swaps the body without re-opening — used by the
//     rail tabs. Pushes the new URL via history.pushState so back/forward
//     navigate between visited tabs naturally.
//   - The dialog's native `close` event fires on Esc, close button, and
//     backdrop click; `onClose()` rewinds history if we pushed it
//     ourselves, else navigates home so the user doesn't land on a blank
//     deep-load host page.
//
// Fragment swap protocol:
//   The server distinguishes a "fragment" request (body only) from a
//   "host" request (full page with modal pre-opened) by checking for the
//   `X-Settings-Fragment: 1` header. Sent on every fetch from here.
document.addEventListener('alpine:init', function () {
  Alpine.data('settingsModal', function () {
    return {
      currentTab: '',
      // tracks how many history entries this instance pushed; when the
      // dialog closes we use this to decide between history.back() and
      // navigating home (no pushes means we landed via a deep-link).
      _pushed: 0,
      // Suppresses the next `close` event's history rewind — used when
      // the dialog closes as part of an intentional swap (rare) so we
      // don't accidentally back out of the modal.
      _suppressNextClose: false,

      init: function () {
        var self = this;
        var initial = this.$el.dataset.initialTab || '';
        var prefilled = this.$el.dataset.initialPrefilled === 'true';

        // Pre-paint the active row before opening so the rail doesn't
        // flash from "Account" to the deep-linked tab.
        if (initial) {
          this.currentTab = initial;
          if (prefilled) {
            // Body is already rendered by the server — just open.
            this.$nextTick(function () {
              self._showModal();
              self._refreshIcons();
              self._consumePageFlashIntoToast();
            });
          } else {
            // No pre-rendered body — fetch it and then open.
            this._loadTab(initial).then(function () {
              self._showModal();
            }).catch(function () {
              // Even on failure, open the dialog so the user sees the
              // error message we surfaced in the body slot.
              self._showModal();
            });
          }
        }

        // Browser back/forward: keep modal state in sync with the URL.
        // When the user hits back from a /settings/* URL we pushed, the
        // popstate target is the prior page — close the dialog.
        // When the user goes forward to a /settings/* URL, re-open.
        window.addEventListener('popstate', function () {
          self._onPopState();
        });

        // Intercept POST submits originating from tabs in this dialog so
        // settings forms don't trigger a full page navigation (which
        // would (a) render the flash banner outside the modal, (b) reset
        // our history-rewind counter, breaking the X button). Fragment
        // POSTs round-trip through the same `X-Settings-Fragment`
        // protocol used for tab loads.
        this.$el.addEventListener('submit', function (e) {
          self._onSubmit(e);
        });
      },

      // Opens the modal at the given tab. Used by the sidebar gear and
      // any other entry point that dispatches `open-settings`.
      open: function (tab) {
        var target = tab || 'account';
        if (this.$el.open && this.currentTab === target) return;
        var self = this;
        this._loadTab(target).then(function () {
          self._pushUrl(target, /*replace=*/ false);
          self._pushed += 1;
          self._showModal();
        }).catch(function () {
          self._pushUrl(target, false);
          self._pushed += 1;
          self._showModal();
        });
      },

      // Switches tabs inside an already-open modal. No dialog open/close
      // churn — just a body swap + URL push.
      switchTo: function (tab) {
        if (!tab) return;
        // Collapse the mobile dropdown the moment a row is tapped — even
        // when the tap re-selects the current tab — so the user gets
        // instant feedback instead of staring at the still-open list.
        this._closeMobileDropdown();
        if (tab === this.currentTab) return;
        var self = this;
        this._loadTab(tab).then(function () {
          self._pushUrl(tab, /*replace=*/ false);
          self._pushed += 1;
        });
      },

      onClose: function () {
        if (this._suppressNextClose) {
          this._suppressNextClose = false;
          return;
        }
        // Rewind the URL pushes this instance made, so we land back on
        // the page the user was actually on. If we never pushed (cold
        // deep-load with no prior history we own), go home — the user
        // explicitly tapped Close on a /settings/* URL, so dumping them
        // on whatever happened to be one slot back in browser history
        // (often another /settings/* page that would just re-open the
        // modal) is confusing.
        if (this._pushed > 0) {
          var steps = this._pushed;
          this._pushed = 0;
          history.go(-steps);
        } else {
          window.location.replace('/');
        }
      },

      // -------- helpers --------

      // currentTabLabel / currentTabIcon — mobile summary uses these to
      // mirror the active row's label + icon without a second source of
      // truth.
      currentTabLabel: function () {
        return TAB_META[this.currentTab] ? TAB_META[this.currentTab].label : 'Settings';
      },
      currentTabIcon: function () {
        return TAB_META[this.currentTab] ? TAB_META[this.currentTab].icon : 'settings';
      },

      _showModal: function () {
        if (this.$el.open) return;
        try { this.$el.showModal(); } catch (_) { /* already open */ }
        this._closeMobileDropdown();
      },

      _closeMobileDropdown: function () {
        var d = this.$refs.mobileDropdown;
        if (d && d.tagName === 'DETAILS') d.open = false;
      },

      _pushUrl: function (tab, replace) {
        var url = '/settings/' + tab;
        if (replace) history.replaceState({ bbSettings: tab }, '', url);
        else history.pushState({ bbSettings: tab }, '', url);
      },

      _onPopState: function () {
        // Read the new URL and decide whether the modal should be open.
        var match = window.location.pathname.match(/^\/settings(?:\/([\w-]+))?\/?$/);
        if (match) {
          var tab = match[1] || 'account';
          var self = this;
          if (this.currentTab !== tab) {
            this._loadTab(tab).then(function () {
              if (!self.$el.open) self._showModal();
            });
          }
        } else if (this.$el.open) {
          // We've left the /settings/* URL space — close the modal but
          // don't try to rewind history again (the browser already did).
          this._suppressNextClose = true;
          this.$el.close();
        }
      },

      _loadTab: function (tab) {
        var self = this;
        var bodyEl = this.$refs.body || document.getElementById('bb-settings-body');
        if (!bodyEl) return Promise.reject(new Error('no settings body slot'));
        // Render a lightweight loading state — daisy skeleton blocks
        // matching the section card shape.
        bodyEl.innerHTML = SKELETON_HTML;
        this.currentTab = tab;
        this._refreshIcons();

        return fetch('/settings/' + tab, {
          credentials: 'same-origin',
          headers: { 'X-Settings-Fragment': '1', 'Accept': 'text/html' },
        }).then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          // Tab GETs don't currently set X-BB-Flash-*, but it costs
          // nothing to forward in case a future handler does.
          self._showFlashFromHeaders(res.headers);
          return res.text();
        }).then(function (html) {
          return self._swapBody(html, tab);
        }).catch(function (err) {
          bodyEl.innerHTML = '<div class="alert alert-error rounded-xl text-sm">' +
            'Could not load settings: ' + (err && err.message ? err.message : 'unknown error') +
            '</div>';
        });
      },

      // _onSubmit handles every form submit dispatched inside the
      // dialog. Forms living in #bb-settings-body get hijacked into a
      // fragment-POST so the modal stays open and feedback renders as
      // an in-dialog toast instead of as a page-level banner. Forms
      // outside the body slot (the close button's <form method="dialog">
      // and the modal-backdrop button) are left to the browser.
      _onSubmit: function (e) {
        var form = e.target;
        if (!form || form.tagName !== 'FORM') return;
        if (form.method && form.method.toLowerCase() === 'dialog') return;
        var bodyEl = this.$refs.body || document.getElementById('bb-settings-body');
        if (!bodyEl || !bodyEl.contains(form)) return;
        // Only POSTs flow through here. GET forms (search filters etc.)
        // belong to whatever page they sit on.
        var method = (form.method || 'GET').toUpperCase();
        if (method !== 'POST') return;

        e.preventDefault();
        this._submitForm(form);
      },

      _submitForm: function (form) {
        var self = this;
        var action = form.action || window.location.href;
        var data = new FormData(form);
        // Disable submit buttons so a frantic double-click doesn't
        // double-post. We re-enable on completion (success or error) —
        // on success the buttons usually get replaced by the body swap
        // anyway, but on the keep-current-body error path they need to
        // come back.
        var buttons = Array.prototype.slice.call(form.querySelectorAll('button, input[type=submit]'));
        buttons.forEach(function (b) { b.disabled = true; });
        var reenable = function () {
          buttons.forEach(function (b) { b.disabled = false; });
        };

        return fetch(action, {
          method: 'POST',
          body: data,
          credentials: 'same-origin',
          // `redirect: 'follow'` (default) so the server's 303-after-POST
          // round-trips through the GET handler, which honors the
          // fragment header and returns just the tab body.
          headers: { 'X-Settings-Fragment': '1', 'Accept': 'text/html' },
        }).then(function (res) {
          var serverToldUs = !!res.headers.get('X-BB-Flash-Message');
          self._showFlashFromHeaders(res.headers);
          // If the redirect target sits outside the settings shell
          // (password change → /login, etc.), fall back to a hard
          // navigation so the user actually lands on that page.
          var finalUrl = res.url || action;
          var dest = parseSettingsRedirect(finalUrl);
          if (!dest.settings) {
            window.location.href = finalUrl;
            return null;
          }
          // Bare /settings redirects (used by Sync + Retention) are
          // ambiguous about the tab — fall back to whichever tab the
          // user was on, since that's also what the server rendered
          // into the fragment.
          var targetTab = dest.tab || self.currentTab;
          if (!res.ok) {
            // Server returned an error fragment (validation failure,
            // 500, etc.). Surface it inside the modal — body still
            // contains the form so the user can retry.
            reenable();
            if (!serverToldUs) {
              self._showToast('error', 'Save failed (HTTP ' + res.status + ').');
            }
            return res.text().then(function (html) {
              return self._swapBody(html, targetTab);
            });
          }
          return res.text().then(function (html) {
            return self._swapBody(html, targetTab).then(function () {
              // Keep the URL bar in sync only when the server explicitly
              // named a different tab. Bare /settings redirects leave
              // the URL alone — the visible tab didn't change.
              if (dest.tab && window.location.pathname !== '/settings/' + dest.tab) {
                self._pushUrl(dest.tab, false);
                self._pushed += 1;
              }
            });
          });
        }).catch(function (err) {
          reenable();
          self._showToast('error', 'Save failed: ' + (err && err.message ? err.message : 'network error'));
        });
      },

      // _swapBody replaces #bb-settings-body with a freshly-fetched
      // fragment, preserving the same alpine:init re-dispatch dance
      // _loadTab used to use inline. Extracted so the form-submit path
      // gets identical script-load semantics.
      _swapBody: function (html, tab) {
        var self = this;
        var bodyEl = this.$refs.body || document.getElementById('bb-settings-body');
        if (!bodyEl) return Promise.reject(new Error('no settings body slot'));

        // Parse the fragment OFF-DOM so Alpine's mutation observer
        // doesn't try to instantiate x-data="avatarEditor" before the
        // tab's factory script has loaded. We pull the <script src=…>
        // tags out, load them via <head>, and only THEN drop the
        // (script-free) HTML into the live body where Alpine wires it
        // up against the now-registered factories.
        var template = document.createElement('template');
        template.innerHTML = html;
        var frag = template.content;
        var srcs = [];
        frag.querySelectorAll('script[src]').forEach(function (s) {
          srcs.push(s.getAttribute('src'));
        });
        // Strip both inline and external scripts from the fragment —
        // we'll handle execution ourselves. Inline scripts inside tab
        // bodies are tiny (a few lines of immediate-effect code like
        // hash-scroll); we re-execute them after the body is mounted.
        var inlineSources = [];
        frag.querySelectorAll('script').forEach(function (s) {
          if (!s.hasAttribute('src')) inlineSources.push(s.textContent);
          s.parentNode.removeChild(s);
        });

        return self._loadScripts(srcs).then(function () {
          bodyEl.replaceChildren(frag);
          self.currentTab = tab;
          inlineSources.forEach(function (src) {
            try { (0, eval)(src); } catch (e) { console.warn('settings tab inline script error:', e); }
          });
          self._refreshIcons();
          var scrollWrap = bodyEl.closest('.overflow-y-auto');
          if (scrollWrap) scrollWrap.scrollTop = 0;
        });
      },

      // _showFlashFromHeaders renders a toast for the response if the
      // server set X-BB-Flash-*. Empty / missing headers no-op. The
      // type defaults to "info" so an empty type still surfaces, but
      // we explicitly tolerate the success/error/warning trio.
      _showFlashFromHeaders: function (headers) {
        var type = headers.get('X-BB-Flash-Type') || '';
        var raw = headers.get('X-BB-Flash-Message') || '';
        if (!raw) return;
        // Go's url.QueryEscape (server side) encodes spaces as `+`,
        // which decodeURIComponent ignores — swap them to %20 first so
        // the round-trip survives.
        var message;
        try { message = decodeURIComponent(raw.replace(/\+/g, '%20')); } catch (_) { message = raw; }
        this._showToast(type, message);
      },

      // _consumePageFlashIntoToast moves the layout's flash banner
      // (rendered inside <main> by html/template on cold deep-loads)
      // into the in-modal toast, then hides the banner. Without this,
      // cold-loading /settings/sync right after a redirect-with-flash
      // would show the flash behind the modal AND we'd lose it once
      // the modal closes.
      _consumePageFlashIntoToast: function () {
        var banner = document.querySelector('main [data-bb-flash]');
        if (!banner) return;
        var type = banner.getAttribute('data-bb-flash') || '';
        var message = (banner.getAttribute('data-bb-flash-message') || banner.textContent || '').trim();
        if (!message) return;
        this._showToast(type, message);
        banner.remove();
      },

      // _showToast mounts an alert inside the dialog's toast slot. The
      // success toast auto-dismisses; error/warning stay until the
      // user dismisses them.
      _showToast: function (type, message) {
        var slot = this.$refs.toast;
        if (!slot) return;
        var tone = ({
          success: 'alert-success',
          error:   'alert-error',
          warning: 'alert-warning',
        })[type] || 'alert-info';
        // Clear any previous toast — only one shown at a time keeps the
        // dialog uncluttered.
        slot.replaceChildren();
        var node = document.createElement('div');
        node.className = 'alert ' + tone + ' alert-soft rounded-xl shadow-lg pointer-events-auto max-w-md text-sm';
        node.setAttribute('role', type === 'error' ? 'alert' : 'status');
        node.textContent = message;
        slot.appendChild(node);
        if (type !== 'error' && type !== 'warning') {
          var t = setTimeout(function () {
            if (node.isConnected) node.remove();
          }, 4000);
          node.addEventListener('click', function () {
            clearTimeout(t);
            node.remove();
          });
        } else {
          node.addEventListener('click', function () { node.remove(); });
        }
      },

      // _loadScripts loads a list of external script URLs in parallel,
      // skipping any that are already on the page. Returns a Promise
      // that resolves when every script has finished loading (or failed
      // — we resolve on error too so a single 404 doesn't deadlock the
      // tab swap). Used by _loadTab to preload tab factories BEFORE
      // injecting the tab body, sidestepping the Alpine mutation-observer
      // race.
      _loadScripts: function (srcs) {
        var newOnes = srcs.filter(function (src) { return !BB_LOADED_SCRIPTS[src]; });
        var jobs = newOnes.map(function (src) {
          return new Promise(function (resolve) {
            var s = document.createElement('script');
            s.src = src;
            s.onload = function () { BB_LOADED_SCRIPTS[src] = true; resolve(); };
            s.onerror = function () { resolve(); };
            document.head.appendChild(s);
          });
        });
        return Promise.all(jobs).then(function () {
          if (newOnes.length === 0) return;
          // Convention in this codebase: every tab JS registers its
          // factories inside `document.addEventListener('alpine:init', …)`.
          // Alpine fires that event ONCE at startup; by the time we
          // dynamically load a tab script, the event has already passed,
          // so the listener would never run. Re-dispatch the event so
          // freshly-loaded scripts get to wire up. Re-dispatch is safe —
          // Alpine.data(name, fn) and Alpine.store(name, obj) are both
          // idempotent by name, and no listener in this codebase has a
          // side effect that wouldn't survive being called twice.
          document.dispatchEvent(new CustomEvent('alpine:init'));
        });
      },

      _refreshIcons: function () {
        if (typeof lucide === 'undefined') return;
        try { lucide.createIcons({ nodes: [this.$el] }); } catch (_) { /* ignore */ }
      },
    };
  });
});

// Tab metadata mirror of the templ rail — kept in sync so the mobile
// summary can render the active label/icon without a server round-trip.
var TAB_META = {
  'account':   { label: 'Account',   icon: 'user-cog' },
  'sync':      { label: 'Sync',      icon: 'refresh-cw' },
  'security':  { label: 'Security',  icon: 'shield' },
  'system':    { label: 'System',    icon: 'cpu' },
  'providers': { label: 'Providers', icon: 'plug' },
  'agents':    { label: 'Agents',    icon: 'sparkles' },
  'mcp':       { label: 'MCP',       icon: 'bot' },
  'api-keys':  { label: 'API Keys',  icon: 'key-round' },
  'backups':   { label: 'Backups',   icon: 'hard-drive' },
  'help':      { label: 'Help',      icon: 'life-buoy' },
};

// Tracks which tab factory scripts have been loaded into the page so
// we don't double-fetch them on repeated tab switches. Module-scoped
// so it survives across Alpine.data factory invocations.
var BB_LOADED_SCRIPTS = {};

// parseSettingsRedirect classifies a fetch response URL relative to the
// settings shell. Used by the form-submit interceptor to decide between
// an in-modal body swap and a hard navigation.
//
// Returns an object:
//   { settings: true,  tab: 'sync' }   — URL is /settings/<tab>
//   { settings: true,  tab: null    }  — URL is bare /settings (ambiguous;
//                                        keep the current tab + URL)
//   { settings: false }                — URL is outside the shell; the
//                                        caller should hard-navigate.
function parseSettingsRedirect(rawUrl) {
  try {
    var u = new URL(rawUrl, window.location.href);
    if (u.origin !== window.location.origin) return { settings: false };
    var m = u.pathname.match(/^\/settings(?:\/([\w-]+))?\/?$/);
    if (!m) return { settings: false };
    var tab = m[1] || null;
    if (tab && !TAB_META[tab]) return { settings: false };
    return { settings: true, tab: tab };
  } catch (_) {
    return { settings: false };
  }
}

// Skeleton placeholder while a tab fragment is in flight — daisy
// `skeleton` blocks shaped roughly like the section-card pattern.
var SKELETON_HTML = [
  '<div class="space-y-4">',
  '  <div class="skeleton h-6 w-40 rounded-md"></div>',
  '  <div class="skeleton h-32 w-full rounded-2xl"></div>',
  '  <div class="skeleton h-24 w-full rounded-2xl"></div>',
  '</div>',
].join('');
