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
        if (!tab || tab === this.currentTab) return;
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
        // deep-load with no prior history we own), navigate home so the
        // user isn't stranded on a blank /settings/* URL.
        if (this._pushed > 0) {
          var steps = this._pushed;
          this._pushed = 0;
          history.go(-steps);
        } else {
          // Cold deep-load — if the browser has any prior history, use
          // it; otherwise drop to /.
          if (window.history.length > 1) {
            history.back();
          } else {
            window.location.replace('/');
          }
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
          return res.text();
        }).then(function (html) {
          bodyEl.innerHTML = html;
          self.currentTab = tab;
          // Execute any inline <script> tags in the fragment so the tab's
          // Alpine factories / lucide refreshers wire up. innerHTML alone
          // doesn't run scripts.
          self._executeScripts(bodyEl);
          self._refreshIcons();
          // Scroll body to top — feels jarring otherwise when switching
          // from a long tab to a short one.
          var scrollWrap = bodyEl.closest('.overflow-y-auto');
          if (scrollWrap) scrollWrap.scrollTop = 0;
        }).catch(function (err) {
          bodyEl.innerHTML = '<div class="alert alert-error rounded-xl text-sm">' +
            'Could not load settings: ' + (err && err.message ? err.message : 'unknown error') +
            '</div>';
        });
      },

      _executeScripts: function (container) {
        // Re-insert each <script> as a fresh node so the browser runs it.
        // External scripts (src=...) get a separate node so their load
        // order is preserved.
        var scripts = container.querySelectorAll('script');
        for (var i = 0; i < scripts.length; i++) {
          var s = scripts[i];
          var fresh = document.createElement('script');
          for (var j = 0; j < s.attributes.length; j++) {
            var a = s.attributes[j];
            fresh.setAttribute(a.name, a.value);
          }
          if (s.src) {
            // dedupe — if this src is already on the page, skip.
            if (document.querySelector('script[src="' + s.src + '"]')) {
              s.parentNode.removeChild(s);
              continue;
            }
          } else {
            fresh.textContent = s.textContent;
          }
          s.parentNode.replaceChild(fresh, s);
        }
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

// Skeleton placeholder while a tab fragment is in flight — daisy
// `skeleton` blocks shaped roughly like the section-card pattern.
var SKELETON_HTML = [
  '<div class="space-y-4">',
  '  <div class="skeleton h-6 w-40 rounded-md"></div>',
  '  <div class="skeleton h-32 w-full rounded-2xl"></div>',
  '  <div class="skeleton h-24 w-full rounded-2xl"></div>',
  '</div>',
].join('');
