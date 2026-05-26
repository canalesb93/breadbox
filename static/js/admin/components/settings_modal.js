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

      // _pushPath is the multi-segment variant — used by the form-submit
      // interceptor when the server redirects to a subpath like
      // /settings/api-keys/{id}/created.
      _pushPath: function (path, tab) {
        history.pushState({ bbSettings: tab }, '', path);
      },

      _onPopState: function () {
        // Read the new URL and decide whether the modal should be open.
        // Accept multi-segment paths (e.g. /settings/api-keys/{id}/created)
        // so back/forward through a creation flow stays in the modal.
        var dest = parseSettingsRedirect(window.location.href);
        if (dest.settings) {
          var tab = dest.tab || 'account';
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
        // Defer tab GETs while a submit is in flight: the server's
        // session flash is one-shot, and an interleaved GET would
        // consume it before the submit's redirect GET arrives, eating
        // the toast that belongs to the save.
        if (self._submitInFlight) {
          return new Promise(function (resolve) {
            var poll = setInterval(function () {
              if (!self._submitInFlight) {
                clearInterval(poll);
                resolve(self._loadTab(tab));
              }
            }, 50);
          });
        }
        // Track the in-flight tab so the swap doesn't clobber a newer
        // user click. We don't paint the skeleton synchronously —
        // sub-120ms responses (warm cache, dev loop) shouldn't show
        // any loading chrome at all.
        this.currentTab = tab;
        var skeletonTimer = setTimeout(function () {
          if (self.currentTab !== tab) return; // user moved on
          bodyEl.innerHTML = bbBuildSettingsSkeleton(tab);
          self._refreshIcons();
        }, 120);

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
          clearTimeout(skeletonTimer);
          return self._swapBody(html, tab);
        }).catch(function (err) {
          clearTimeout(skeletonTimer);
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
        // Bail if the form's own listener already prevented the
        // submit — Alpine's `@submit.prevent` (bb-confirm dialogs on
        // Backups, etc.) cancels the real submit and dispatches a
        // confirmation step that re-invokes `requestSubmit()` on the
        // user's OK. Our listener bubbles after Alpine's; firing
        // _submitForm here would post the form even though Alpine
        // wanted to wait for the confirm.
        if (e.defaultPrevented) return;

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

        // Block parallel tab GETs while this submit is in flight. The
        // server's session flash is one-shot: an interleaved
        // `_loadTab` would consume the flash that belongs to this
        // POST's redirect target, so the user wouldn't see the toast.
        self._submitInFlight = true;
        var clearInFlight = function () { self._submitInFlight = false; };

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
          // Same-tab refresh: skip the inline-script re-eval (so the
          // agents-tab hash-scroll IIFE doesn't snap the viewport on
          // every save) and preserve the modal's scroll position (so
          // the user stays next to the form they just submitted).
          var sameTab = targetTab === self.currentTab;
          var swapOpts = { skipInlineScripts: sameTab, preserveScroll: sameTab };
          if (!res.ok) {
            // Server returned an error fragment (validation failure,
            // 500, etc.). Surface it inside the modal — body still
            // contains the form so the user can retry.
            reenable();
            if (!serverToldUs) {
              self._showToast('error', 'Save failed (HTTP ' + res.status + ').');
            }
            return res.text().then(function (html) {
              return self._swapBody(html, targetTab, swapOpts);
            });
          }
          return res.text().then(function (html) {
            return self._swapBody(html, targetTab, swapOpts).then(function () {
              // Keep the URL bar in sync with the server's redirect
              // target. We push the FULL pathname (not just
              // /settings/<tab>) so multi-segment routes like
              // /settings/api-keys/{id}/created survive in the address
              // bar and back/forward navigates correctly. Bare
              // /settings redirects leave the URL alone — the visible
              // tab didn't change.
              if (dest.tab && window.location.pathname !== dest.path) {
                self._pushPath(dest.path, dest.tab);
                self._pushed += 1;
              }
            });
          });
        }).catch(function (err) {
          reenable();
          self._showToast('error', 'Save failed: ' + (err && err.message ? err.message : 'network error'));
        }).then(clearInFlight, clearInFlight);
      },

      // _swapBody replaces #bb-settings-body with a freshly-fetched
      // fragment, preserving the same alpine:init re-dispatch dance
      // _loadTab used to use inline. Extracted so the form-submit path
      // gets identical script-load semantics.
      //
      // opts.skipInlineScripts skips re-running inline <script> blocks
      //   from the fragment — used on same-tab post-submit refresh so
      //   hash-scroll IIFEs don't snap the viewport on every save.
      // opts.preserveScroll keeps the modal's scroll position intact
      //   across the swap — also used on same-tab refresh so the user
      //   stays next to the form they just submitted, instead of
      //   getting yanked back to the top of the tab.
      _swapBody: function (html, tab, opts) {
        var self = this;
        var bodyEl = this.$refs.body || document.getElementById('bb-settings-body');
        if (!bodyEl) return Promise.reject(new Error('no settings body slot'));
        var skipInline = !!(opts && opts.skipInlineScripts);
        var preserveScroll = !!(opts && opts.preserveScroll);

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
        // hash-scroll); we re-execute them after the body is mounted
        // unless the caller opted out (same-tab refresh).
        var inlineSources = [];
        frag.querySelectorAll('script').forEach(function (s) {
          if (!s.hasAttribute('src')) inlineSources.push(s.textContent);
          s.parentNode.removeChild(s);
        });

        var scrollWrap = bodyEl.closest('.overflow-y-auto');
        var savedScroll = preserveScroll && scrollWrap ? scrollWrap.scrollTop : 0;

        return self._loadScripts(srcs).then(function () {
          bodyEl.replaceChildren(frag);
          self.currentTab = tab;
          if (!skipInline) {
            inlineSources.forEach(function (src) {
              try { (0, eval)(src); } catch (e) { console.warn('settings tab inline script error:', e); }
            });
          }
          self._refreshIcons();
          if (scrollWrap) scrollWrap.scrollTop = preserveScroll ? savedScroll : 0;
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
      // cold-loading /settings/general right after a redirect-with-flash
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
  'general':   { label: 'General',   icon: 'sliders-horizontal' },
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
//   { settings: true,  tab: 'api-keys', path: '/settings/api-keys/abc/created',
//     hash: '' }                              — inside the shell, the first
//                                              segment maps to a rail tab.
//   { settings: true,  tab: null,       path: '/settings', hash: '' }
//                                            — bare /settings (ambiguous;
//                                              caller keeps the current tab).
//   { settings: false }                      — outside the shell; caller
//                                              hard-navigates.
//
// The api-keys tab also serves /settings/oauth-clients/* (a sibling URL
// prefix that renders the same Access page), so we map it to the same
// rail tab. Unknown first segments fall through to {settings:false} so
// a future /settings/<new-prefix> URL we forgot to wire here doesn't
// silently take over an existing tab.
var SETTINGS_SUBPATH_TAB = {
  'oauth-clients': 'api-keys',
};

function parseSettingsRedirect(rawUrl) {
  try {
    var u = new URL(rawUrl, window.location.href);
    if (u.origin !== window.location.origin) return { settings: false };
    var m = u.pathname.match(/^\/settings(?:\/([\w-]+)(?:\/.*)?)?\/?$/);
    if (!m) return { settings: false };
    var first = m[1] || null;
    if (!first) {
      return { settings: true, tab: null, path: u.pathname, hash: u.hash || '' };
    }
    var tab = TAB_META[first] ? first : (SETTINGS_SUBPATH_TAB[first] || null);
    if (!tab) return { settings: false };
    return { settings: true, tab: tab, path: u.pathname, hash: u.hash || '' };
  } catch (_) {
    return { settings: false };
  }
}

// bbBuildSettingsSkeleton returns a placeholder HTML shaped like the
// requested tab — tab header on top, then SettingsSection-shaped
// blocks each carrying SettingsRow-shaped placeholders. Sharing the
// real settings CSS (bb-settings-section, bb-settings-row) means the
// skeleton-to-content swap is positional rather than a layout shift,
// so anything past the 120ms paint guard reads as the same surface
// fading in instead of a separate loading screen.
//
// Section row counts are deliberately approximate — they don't need
// to match the live tab exactly to feel cohesive; the goal is the
// right number of bordered groups at the right height.
function bbBuildSettingsSkeleton(tab) {
  var spec = BB_TAB_SKELETONS[tab] || BB_TAB_SKELETONS._default;
  var parts = ['<div>'];
  parts.push(bbSkeletonHeader());
  if (spec.alert) parts.push(bbSkeletonAlert());
  if (spec.stats) parts.push(bbSkeletonStatsGrid(spec.stats));
  parts.push('<div class="space-y-6">');
  spec.sections.forEach(function (rows) {
    parts.push(bbSkeletonSection(rows));
  });
  parts.push('</div></div>');
  return parts.join('');
}

function bbSkeletonHeader() {
  return [
    '<div class="mb-6">',
    '  <div class="skeleton h-6 w-32 rounded-md"></div>',
    '  <div class="skeleton h-3.5 w-72 max-w-full rounded-md mt-2 opacity-70"></div>',
    '</div>',
  ].join('');
}

function bbSkeletonSection(rowCount) {
  var n = Math.max(1, rowCount || 1);
  var rows = '';
  for (var i = 0; i < n; i++) rows += bbSkeletonRow();
  return [
    '<section class="bb-settings-section">',
    '  <header class="bb-settings-section__header">',
    '    <div class="skeleton w-4 h-4 rounded-md shrink-0"></div>',
    '    <div class="flex-1 min-w-0">',
    '      <div class="skeleton h-3.5 w-28 rounded-md"></div>',
    '      <div class="skeleton h-3 w-56 max-w-full rounded-md mt-1.5 opacity-70"></div>',
    '    </div>',
    '  </header>',
    '  <div class="bb-settings-section__body">',
    rows,
    '  </div>',
    '</section>',
  ].join('');
}

function bbSkeletonRow() {
  return [
    '<div class="bb-settings-row">',
    '  <div class="bb-settings-row__main">',
    '    <div class="skeleton h-3.5 w-32 rounded-md"></div>',
    '    <div class="skeleton h-3 w-48 max-w-full rounded-md mt-2 opacity-70"></div>',
    '  </div>',
    '  <div class="bb-settings-row__control">',
    '    <div class="skeleton h-8 w-28 rounded-lg"></div>',
    '  </div>',
    '</div>',
  ].join('');
}

function bbSkeletonAlert() {
  // Backups tops with a warning alert; mirror its rough shape so the
  // page doesn't reflow when the real alert paints.
  return '<div class="skeleton h-20 w-full rounded-xl mb-6"></div>';
}

function bbSkeletonStatsGrid(n) {
  var tiles = '';
  for (var i = 0; i < n; i++) {
    tiles += '<div class="skeleton h-20 rounded-2xl"></div>';
  }
  return '<div class="grid grid-cols-2 sm:grid-cols-' + n + ' gap-3 mb-6">' + tiles + '</div>';
}

// Row counts per section, per tab. Approximate — see comment on
// bbBuildSettingsSkeleton.
var BB_TAB_SKELETONS = {
  'general':   { sections: [2, 3] },
  'system':    { sections: [3, 5] },
  'help':      { sections: [2, 1] },
  'account':   { sections: [4, 3, 1] },
  'api-keys':  { sections: [3, 4] },
  'backups':   { alert: true, stats: 4, sections: [3, 2, 2] },
  'providers': { sections: [4, 4, 2] },
  'agents':    { sections: [3, 5, 2] },
  'mcp':       { sections: [2, 2, 2, 3, 3] },
  '_default':  { sections: [3, 3] },
};
