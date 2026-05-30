// Alpine factory for the full-page Settings surface (/settings/*).
//
// Settings is a real page now (rail + content inside the app chrome), not
// a modal overlay. This factory keeps tab switching and in-tab saves feeling
// instant by swapping just the content fragment (#bb-settings-body) instead
// of doing a full navigation — the same X-Settings-Fragment protocol the
// server already speaks. Without JS the rail's <a href> links and the forms
// fall back to ordinary navigations, so the page degrades gracefully.
//
// Distilled from the retired settings_modal.js — the dialog open/close,
// backdrop, and history-rewind logic are gone; what remains is the fragment
// swap, the form-submit interception (so saves toast in place), and URL sync.
document.addEventListener('alpine:init', function () {
  Alpine.data('settingsPage', function () {
    return {
      currentTab: '',
      _submitInFlight: false,

      init: function () {
        var self = this;
        this.currentTab = this.$el.dataset.activeTab || 'account';
        // Keep the URL bar in sync with the server's redirect target, and
        // intercept in-tab POSTs so saves swap the body instead of
        // navigating the whole page away.
        this.$el.addEventListener('submit', function (e) {
          self._onSubmit(e);
        });
        // Re-run the deep-link hash scroll once the first paint settles.
        if (typeof bbSettingsScrollToHash === 'function') {
          this.$nextTick(function () { bbSettingsScrollToHash(); });
        }
      },

      // switchTo loads a tab into the content area and syncs the URL +
      // rail highlight. Called by the rail links' @click.prevent.
      switchTo: function (tab) {
        if (!tab || tab === this.currentTab) return;
        var self = this;
        this._loadTab(tab).then(function () {
          self._pushPath('/settings/' + tab);
        });
      },

      // onPopState re-syncs the visible tab when the user navigates
      // browser history between /settings/* URLs.
      onPopState: function () {
        var dest = parseSettingsRedirect(window.location.href);
        if (!dest.settings) return; // left the settings space entirely
        var tab = dest.tab || this.currentTab;
        if (tab === this.currentTab) return;
        // currentTab (set in _loadTab) drives the rail highlight reactively.
        this._loadTab(tab);
      },

      // -------- helpers --------

      _loadTab: function (tab) {
        var self = this;
        var bodyEl = this.$refs.body || document.getElementById('bb-settings-body');
        if (!bodyEl) return Promise.reject(new Error('no settings body slot'));
        if (self._submitInFlight) {
          // The session flash is one-shot; defer a tab GET until any
          // in-flight save's redirect has consumed its own flash.
          return new Promise(function (resolve) {
            var poll = setInterval(function () {
              if (!self._submitInFlight) {
                clearInterval(poll);
                resolve(self._loadTab(tab));
              }
            }, 50);
          });
        }
        this.currentTab = tab;
        // Skeleton only past a 120ms guard — warm swaps shouldn't flash
        // any loading chrome.
        var skeletonTimer = setTimeout(function () {
          if (self.currentTab !== tab) return;
          bodyEl.innerHTML = bbBuildSettingsSkeleton(tab);
          self._refreshIcons();
        }, 120);

        return fetch('/settings/' + tab, {
          credentials: 'same-origin',
          headers: { 'X-Settings-Fragment': '1', 'Accept': 'text/html' },
        }).then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
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

      _onSubmit: function (e) {
        var form = e.target;
        if (!form || form.tagName !== 'FORM') return;
        if (form.method && form.method.toLowerCase() === 'dialog') return;
        var bodyEl = this.$refs.body || document.getElementById('bb-settings-body');
        if (!bodyEl || !bodyEl.contains(form)) return;
        var method = (form.method || 'GET').toUpperCase();
        if (method !== 'POST') return;
        // Respect Alpine @submit.prevent (bb-confirm flows) that already
        // cancelled the native submit.
        if (e.defaultPrevented) return;
        e.preventDefault();
        this._submitForm(form);
      },

      _submitForm: function (form) {
        var self = this;
        var action = form.action || window.location.href;
        var data = new FormData(form);
        var buttons = Array.prototype.slice.call(form.querySelectorAll('button, input[type=submit]'));
        buttons.forEach(function (b) { b.disabled = true; });
        var reenable = function () { buttons.forEach(function (b) { b.disabled = false; }); };

        self._submitInFlight = true;
        var clearInFlight = function () { self._submitInFlight = false; };

        return fetch(action, {
          method: 'POST',
          body: data,
          credentials: 'same-origin',
          headers: { 'X-Settings-Fragment': '1', 'Accept': 'text/html' },
        }).then(function (res) {
          var serverToldUs = !!res.headers.get('X-BB-Flash-Message');
          self._showFlashFromHeaders(res.headers);
          var finalUrl = res.url || action;
          var dest = parseSettingsRedirect(finalUrl);
          // Redirect target outside the settings space (password change →
          // /login, etc.) → hand off to a real navigation.
          if (!dest.settings) {
            window.location.href = finalUrl;
            return null;
          }
          var targetTab = dest.tab || self.currentTab;
          var sameTab = targetTab === self.currentTab;
          var swapOpts = { skipInlineScripts: sameTab, preserveScroll: sameTab };
          if (!res.ok) {
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
              // Sync the URL to the canonical tab path — not the form's
              // action endpoint (e.g. .../api-keys/new) — so a refresh
              // re-GETs the tab rather than re-posting the form.
              var tabPath = '/settings/' + targetTab;
              if (window.location.pathname !== tabPath) {
                self._pushPath(tabPath);
              }
            });
          });
        }).catch(function (err) {
          reenable();
          self._showToast('error', 'Save failed: ' + (err && err.message ? err.message : 'network error'));
        }).then(clearInFlight, clearInFlight);
      },

      _swapBody: function (html, tab, opts) {
        var self = this;
        var bodyEl = this.$refs.body || document.getElementById('bb-settings-body');
        if (!bodyEl) return Promise.reject(new Error('no settings body slot'));
        var skipInline = !!(opts && opts.skipInlineScripts);
        var preserveScroll = !!(opts && opts.preserveScroll);

        var template = document.createElement('template');
        template.innerHTML = html;
        var frag = template.content;
        var srcs = [];
        frag.querySelectorAll('script[src]').forEach(function (s) {
          srcs.push(s.getAttribute('src'));
        });
        var inlineSources = [];
        frag.querySelectorAll('script').forEach(function (s) {
          if (!s.hasAttribute('src')) inlineSources.push(s.textContent);
          s.parentNode.removeChild(s);
        });

        var scroller = document.scrollingElement || document.documentElement;
        var savedScroll = preserveScroll ? scroller.scrollTop : 0;

        return self._loadScripts(srcs).then(function () {
          bodyEl.replaceChildren(frag);
          self.currentTab = tab;
          if (!skipInline) {
            inlineSources.forEach(function (src) {
              try { (0, eval)(src); } catch (e) { console.warn('settings tab inline script error:', e); }
            });
          }
          self._refreshIcons();
          if (preserveScroll) scroller.scrollTop = savedScroll;
          else scroller.scrollTop = 0;
        });
      },

      _pushPath: function (path) {
        history.pushState({ bbSettings: true }, '', path);
      },

      _showFlashFromHeaders: function (headers) {
        var type = headers.get('X-BB-Flash-Type') || '';
        var raw = headers.get('X-BB-Flash-Message') || '';
        if (!raw) return;
        var message;
        try { message = decodeURIComponent(raw.replace(/\+/g, '%20')); } catch (_) { message = raw; }
        this._showToast(type, message);
      },

      _showToast: function (type, message) {
        var slot = this.$refs.toast;
        if (!slot) return;
        var tone = ({
          success: 'alert-success',
          error: 'alert-error',
          warning: 'alert-warning',
        })[type] || 'alert-info';
        slot.replaceChildren();
        var node = document.createElement('div');
        node.className = 'alert ' + tone + ' alert-soft rounded-xl shadow-lg pointer-events-auto max-w-md text-sm';
        node.setAttribute('role', type === 'error' ? 'alert' : 'status');
        node.textContent = message;
        slot.appendChild(node);
        if (type !== 'error' && type !== 'warning') {
          var t = setTimeout(function () { if (node.isConnected) node.remove(); }, 4000);
          node.addEventListener('click', function () { clearTimeout(t); node.remove(); });
        } else {
          node.addEventListener('click', function () { node.remove(); });
        }
      },

      _loadScripts: function (srcs) {
        var newOnes = srcs.filter(function (src) { return !BB_SETTINGS_LOADED_SCRIPTS[src]; });
        var jobs = newOnes.map(function (src) {
          return new Promise(function (resolve) {
            var s = document.createElement('script');
            s.src = src;
            s.onload = function () { BB_SETTINGS_LOADED_SCRIPTS[src] = true; resolve(); };
            s.onerror = function () { resolve(); };
            document.head.appendChild(s);
          });
        });
        return Promise.all(jobs).then(function () {
          if (newOnes.length === 0) return;
          // Tab factories register inside an alpine:init listener that has
          // already fired; re-dispatch so freshly-loaded scripts wire up.
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

// Valid settings tab ids — used by parseSettingsRedirect to classify a
// response URL as inside the settings space.
var BB_SETTINGS_PAGE_TABS = {
  account: 1, general: 1, system: 1, providers: 1, agents: 1,
  mcp: 1, 'api-keys': 1, backups: 1, help: 1,
};

// Sibling URL prefixes that render under an existing rail tab.
var BB_SETTINGS_SUBPATH_TAB = {
  'oauth-clients': 'api-keys',
};

// Tracks loaded tab factory scripts so repeat switches don't re-fetch.
var BB_SETTINGS_LOADED_SCRIPTS = {};

// parseSettingsRedirect classifies a URL relative to the settings space.
//   { settings: true,  tab: 'api-keys', path: '/settings/api-keys' }
//   { settings: true,  tab: null,       path: '/settings' }  (ambiguous)
//   { settings: false }                                       (outside)
function parseSettingsRedirect(rawUrl) {
  try {
    var u = new URL(rawUrl, window.location.href);
    if (u.origin !== window.location.origin) return { settings: false };
    var m = u.pathname.match(/^\/settings(?:\/([\w-]+)(?:\/.*)?)?\/?$/);
    if (!m) return { settings: false };
    var first = m[1] || null;
    if (!first) return { settings: true, tab: null, path: u.pathname, hash: u.hash || '' };
    var tab = BB_SETTINGS_PAGE_TABS[first] ? first : (BB_SETTINGS_SUBPATH_TAB[first] || null);
    if (!tab) return { settings: false };
    return { settings: true, tab: tab, path: u.pathname, hash: u.hash || '' };
  } catch (_) {
    return { settings: false };
  }
}

// bbBuildSettingsSkeleton returns placeholder HTML shaped like a settings
// tab (page header + borderless two-column section blocks) so the swap
// reads as content fading in rather than a separate loading screen.
function bbBuildSettingsSkeleton(tab) {
  var spec = BB_SETTINGS_SKELETONS[tab] || BB_SETTINGS_SKELETONS._default;
  var parts = ['<div>', bbSettingsSkeletonHeader()];
  spec.sections.forEach(function (rows) { parts.push(bbSettingsSkeletonSection(rows)); });
  parts.push('</div>');
  return parts.join('');
}

function bbSettingsSkeletonHeader() {
  return [
    '<div class="pb-6 mb-2 border-b border-base-200">',
    '  <div class="skeleton h-7 w-36 rounded-md"></div>',
    '  <div class="skeleton h-4 w-72 max-w-full rounded-md mt-2 opacity-70"></div>',
    '</div>',
  ].join('');
}

function bbSettingsSkeletonSection(rowCount) {
  var n = Math.max(1, rowCount || 1);
  var rows = '';
  for (var i = 0; i < n; i++) rows += bbSettingsSkeletonRow();
  return [
    '<div class="bb-settings-section">',
    '  <div class="bb-settings-section__grid">',
    '    <div class="bb-settings-section__meta">',
    '      <div class="skeleton h-4 w-28 rounded-md"></div>',
    '      <div class="skeleton h-3 w-40 max-w-full rounded-md mt-2 opacity-70"></div>',
    '    </div>',
    '    <div class="bb-settings-section__content">' + rows + '</div>',
    '  </div>',
    '</div>',
  ].join('');
}

function bbSettingsSkeletonRow() {
  return [
    '<div class="bb-settings-row">',
    '  <div class="bb-settings-row__main">',
    '    <div class="skeleton h-3.5 w-32 rounded-md"></div>',
    '  </div>',
    '  <div class="bb-settings-row__control">',
    '    <div class="skeleton h-8 w-28 rounded-lg"></div>',
    '  </div>',
    '</div>',
  ].join('');
}

var BB_SETTINGS_SKELETONS = {
  general: { sections: [2, 2] },
  system: { sections: [3, 4] },
  help: { sections: [2, 1] },
  account: { sections: [3, 1, 1] },
  'api-keys': { sections: [3, 3] },
  backups: { sections: [3, 2, 2] },
  providers: { sections: [3, 3, 2] },
  agents: { sections: [3, 4, 2] },
  mcp: { sections: [2, 2, 2, 3] },
  _default: { sections: [3, 3] },
};
