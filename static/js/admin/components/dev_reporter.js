// Developer Mode reporter — the always-on-top bug/task filer rendered (gated
// by .DevModeEnabled) at the end of base.html. On open it screenshots the
// current viewport with html2canvas (lazy-loaded from CDN on first use) and
// captures an HTML snapshot, then shows a small form. Submit POSTs the
// artifacts + page metadata to /-/dev-reports, which hosts them on the
// artifact store (bb-artifacts.exe.xyz) and returns a prefilled GitHub
// issue-draft URL (image embedded, snapshot linked) that the client opens for
// the user to submit. CSRF is added by the global fetch wrapper in base.html.
// See internal/admin/dev_reports.go.
//
// PRIVACY: by default ("Redact financial data" on) every capture is masked so
// no real transaction data leaves the instance. Redaction runs on a CLONE of
// the page (html2canvas `onclone` for the image; a detached clone for the
// HTML) — the live page is never mutated. The HTML snapshot ALWAYS strips
// scripts, the CSRF token, and input values regardless of the toggle, because
// those are code/secrets rather than "data the user chose to share".

(function () {
  // App-shell elements whose text stays readable (never financial data) so a
  // reviewer can still tell which page/area the report is about. Deliberately
  // precise — NOT a bare `header`/`nav`, because Breadbox uses semantic
  // <header>/<nav> inside page content too. Everything outside this allowlist
  // (main content, overlays, drawers, modals) is redacted.
  var REDACT_CHROME = '#bb-dev-reporter, [data-redact="false"], .bb-topbar, .bb-sidebar, .bb-mobile-navbar, .bb-settings-rail';

  function bbClosestChrome(el) {
    return !!(el && el.closest && el.closest(REDACT_CHROME));
  }

  // Replace letters and digits with • while keeping whitespace and punctuation,
  // so the masked text keeps roughly the same shape and length.
  function bbMaskText(s) {
    return s.replace(/[\p{L}\p{N}]/gu, '•');
  }

  // Email addresses are PII that can live in app chrome (the user menu, the
  // sidebar footer) — scrub them everywhere, even where the rest of the text
  // stays readable.
  var BB_EMAIL_RE = /[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}/g;
  function bbScrubPII(s) { return s.replace(BB_EMAIL_RE, '•••@•••'); }

  // Walk every text node under root and redact according to its marking:
  //
  //   [data-private]      — financial data already handled by bbGlitchPrivate
  //                         (shape-preserving glitch); skipped here when
  //                         skipPrivate=true.
  //   [data-user-content] — opt-in marker for other user-generated text (notes,
  //                         free-form descriptions, custom names); fully dot-
  //                         masked here. Convention: add data-user-content to
  //                         any user-entered text that isn't already covered by
  //                         data-private (amounts, merchants, accounts, etc.).
  //   chrome selectors    — nav/sidebar/topbar labels that stay readable but
  //                         have email-style PII scrubbed.
  //   everything else     — static UI text (labels, headings, system category
  //                         names, button text); PII-scrubbed only, never blanked.
  //
  // skipPrivate is only set when the glitch pass actually ran; a missing
  // privacy engine safely falls back to dot-masking the data-user-content nodes.
  function bbMaskTextNodes(root, skipPrivate) {
    var doc = root.ownerDocument || document;
    var walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: function (node) {
        if (!node.nodeValue || !node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
        var p = node.parentElement;
        if (!p) return NodeFilter.FILTER_REJECT;
        var tag = p.nodeName;
        if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'TITLE' || tag === 'NOSCRIPT') return NodeFilter.FILTER_REJECT;
        if (skipPrivate && p.closest && p.closest('[data-private]')) return NodeFilter.FILTER_REJECT;
        return NodeFilter.FILTER_ACCEPT;
      },
    });
    var n, batch = [];
    while ((n = walker.nextNode())) batch.push(n);
    batch.forEach(function (node) {
      var p = node.parentElement;
      if (bbClosestChrome(p)) {
        // App chrome: scrub PII only (emails, seeds) — labels stay readable.
        node.nodeValue = bbScrubPII(node.nodeValue);
      } else if (p && p.closest && p.closest('[data-user-content]')) {
        // Explicitly opted-in user-generated text: full dot masking.
        node.nodeValue = bbMaskText(node.nodeValue);
      } else {
        // Static UI text (headings, system labels, category names, etc.):
        // scrub PII only — never blank static strings that help a reviewer
        // understand which page/area the report is about.
        node.nodeValue = bbScrubPII(node.nodeValue);
      }
    });
  }

  // Neutralise raster/media that text-masking can't reach: <img> (avatars,
  // logos, receipts) and <canvas> (charts). SVG charts are handled by the text
  // walker (their <text> labels get masked); SVG icons aren't sensitive.
  function bbHideMedia(root) {
    root.querySelectorAll('img, canvas').forEach(function (el) {
      if (bbClosestChrome(el)) return;
      el.style.setProperty('visibility', 'hidden', 'important');
    });
    root.querySelectorAll('[style*="background-image"]').forEach(function (el) {
      if (bbClosestChrome(el)) return;
      el.style.setProperty('background-image', 'none', 'important');
    });
  }

  // Attributes that commonly mirror the visible (sensitive) content — a row's
  // aria-label/title often repeats the merchant + amount, alt repeats a name.
  var BB_DATA_ATTRS = ['title', 'alt', 'aria-label', 'aria-description', 'data-tip', 'placeholder'];

  // Strip data-bearing attributes from non-chrome elements, and drop any
  // custom data-* (which can carry raw values) other than the redaction marker.
  function bbStripDataAttrs(root) {
    root.querySelectorAll('*').forEach(function (el) {
      if (bbClosestChrome(el)) return;
      BB_DATA_ATTRS.forEach(function (a) { if (el.hasAttribute(a)) el.removeAttribute(a); });
      Array.prototype.slice.call(el.attributes || []).forEach(function (a) {
        if (a.name.indexOf('data-') === 0 && a.name !== 'data-redact') el.removeAttribute(a.name);
      });
    });
  }

  // Scrub email addresses out of every remaining attribute value (mailto:
  // hrefs, user-menu titles, avatar seeds, etc.) — even in chrome.
  function bbScrubAttrPII(root) {
    root.querySelectorAll('*').forEach(function (el) {
      Array.prototype.slice.call(el.attributes || []).forEach(function (a) {
        var nv = bbScrubPII(a.value);
        if (nv !== a.value) { try { el.setAttribute(a.name, nv); } catch (e) {} }
      });
    });
  }

  // Obfuscate the financial values (everything tagged data-private — amounts,
  // accounts, merchants, balances) with the shared privacy-mode glitch, on the
  // CLONE only. Returns true when it ran so the dot-masker can skip those nodes
  // and leave the legible same-shape glitch in place. Reads the real value off
  // the clone regardless of the user's live privacy mode, so it never leaks.
  function bbGlitchPrivate(root) {
    try {
      if (window.bbPrivacy && typeof window.bbPrivacy.glitchTree === 'function') {
        window.bbPrivacy.glitchTree(root);
        return true;
      }
    } catch (e) {}
    return false;
  }

  // Redact a cloned document/element in place: glitch financial values, mask
  // the remaining data text, hide media, strip data-bearing attributes, and
  // scrub PII from attributes. The glitch must run BEFORE bbStripDataAttrs
  // (which removes data-private) and before bbMaskTextNodes (which would
  // otherwise dot-mask the glitched values back into •).
  function bbRedactClone(root) {
    var glitched = bbGlitchPrivate(root);
    bbMaskTextNodes(root, glitched);
    bbHideMedia(root);
    bbStripDataAttrs(root);
    bbScrubAttrPII(root);
  }

  // Fetch every same-origin stylesheet's text once and cache it. Two call sites
  // need the raw CSS:
  //   1. The screenshot — html2canvas clones the page into an iframe that
  //      re-fetches the external <link href="/static/css/styles.css">. On a cold
  //      run those rules can still be unapplied when it rasterizes, producing an
  //      UNSTYLED capture (default margins, blue underlined links). Injecting the
  //      CSS as an inline <style> in the clone removes that race entirely.
  //   2. The HTML snapshot — opened standalone from the artifact store, its
  //      /static/* <link> resolves off-origin and 404s, so the page renders
  //      unstyled. Inlining the CSS makes the snapshot self-contained.
  var _bbCssPromise = null;
  function bbFetchAppCss() {
    if (_bbCssPromise) return _bbCssPromise;
    _bbCssPromise = (function () {
      var links = Array.prototype.slice.call(
        document.querySelectorAll('link[rel="stylesheet"]')
      ).filter(function (l) {
        try { return new URL(l.href, location.href).origin === location.origin; }
        catch (e) { return false; }
      });
      return Promise.all(links.map(function (l) {
        return fetch(l.href).then(function (r) { return r.ok ? r.text() : ''; }).catch(function () { return ''; });
      })).then(function (parts) { return parts.join('\n'); }).catch(function () { return ''; });
    })();
    return _bbCssPromise;
  }

  // Replace the same-origin stylesheet <link>s on a cloned document with a single
  // inline <style> carrying the fetched CSS, so the clone renders styled with no
  // external fetch. No-op when css is empty (keeps the original <link> fallback).
  function bbInlineCss(root, css) {
    if (!css) return;
    try {
      root.querySelectorAll('link[rel="stylesheet"]').forEach(function (el) {
        try { if (new URL(el.href, location.href).origin === location.origin) el.remove(); } catch (e) {}
      });
      var doc = root.ownerDocument || document;
      var head = root.querySelector ? (root.querySelector('head') || root) : root;
      var st = doc.createElement('style');
      st.textContent = css;
      head.appendChild(st);
    } catch (e) {}
  }

  // Build the HTML snapshot string. ALWAYS strips scripts, the CSRF meta, the
  // reporter widget, and input values (code/secrets). Inlines the app CSS (css)
  // so the snapshot renders styled standalone. When redact=true it also masks
  // visible data text, hides media, and clears image sources.
  function bbBuildSnapshot(redact, css) {
    try {
      var clone = document.documentElement.cloneNode(true);
      // Always strip code + secrets.
      clone.querySelectorAll('script, noscript, template, meta[name="csrf-token"], #bb-dev-reporter')
        .forEach(function (el) { el.remove(); });
      clone.querySelectorAll('input, textarea').forEach(function (el) {
        el.removeAttribute('value');
        if ('value' in el) { try { el.value = ''; } catch (e) {} }
      });
      if (redact) {
        bbRedactClone(clone);
        clone.querySelectorAll('img').forEach(function (el) { el.removeAttribute('src'); el.removeAttribute('srcset'); });
      }
      // Inline CSS after redaction so the stylesheet text is never masked.
      bbInlineCss(clone, css);
      var html = '<!DOCTYPE html>\n' + clone.outerHTML;
      var cap = 3000000;
      return html.length > cap ? html.slice(0, cap) : html;
    } catch (e) {
      return '';
    }
  }

  document.addEventListener('alpine:init', function () {
    Alpine.data('devReporter', function () {
      return {
        panelOpen: false,
        type: 'bug',
        title: '',
        description: '',
        screenshot: '',          // data URL of the (possibly redacted) capture
        includeScreenshot: true,
        redact: true,            // privacy: mask financial data (default on)
        capturing: false,
        captureNote: '',
        submitting: false,
        error: '',
        pagePath: location.pathname + location.search,
        page: '',
        user: '',
        version: '',
        _h2cPromise: null,
        _snapshot: '',

        init: function () {
          var ds = this.$root.dataset || {};
          this.page = ds.page || '';
          this.user = ds.user || '';
          this.version = ds.version || '';
          // Remember the user's redaction preference across reports.
          try {
            var pref = localStorage.getItem('bb-dev-redact');
            if (pref === '0') this.redact = false;
          } catch (e) {}

          var reg = Alpine.store('shortcuts');
          if (reg && typeof reg.register === 'function') {
            var self = this;
            reg.register({
              id: 'dev.report',
              keys: 'g b',
              description: 'Report a bug or task',
              group: 'Developer',
              scope: 'global',
              action: function () { if (!self.panelOpen) self.open(); },
            });
          }
        },

        open: function () {
          this.error = '';
          this.captureNote = '';
          this.screenshot = '';
          this.capturing = true; // show the "Preparing…" spinner immediately
          this.pagePath = location.pathname + location.search;
          // Open the panel right away so there's instant feedback, then run the
          // (heavier) capture after a paint so the spinner is actually visible.
          this.panelOpen = true;
          var self = this;
          this.$nextTick(function () {
            if (self.$refs.titleInput) self.$refs.titleInput.focus();
            setTimeout(function () { self.recapture(); }, 60);
          });
        },

        close: function () { this.panelOpen = false; },

        reset: function () {
          this.type = 'bug';
          this.title = '';
          this.description = '';
          this.screenshot = '';
          this.error = '';
          this.captureNote = '';
          this._snapshot = '';
        },

        // Persist the redaction preference and re-capture so the preview + the
        // pending snapshot reflect the new setting.
        onRedactChange: function () {
          try { localStorage.setItem('bb-dev-redact', this.redact ? '1' : '0'); } catch (e) {}
          this.recapture();
        },

        // recapture builds both artifacts (screenshot + HTML snapshot) honoring
        // the current redact setting. It resolves the app CSS once up front so
        // both the capture and the snapshot render with the page's real styles.
        recapture: function () {
          var self = this;
          this.capturing = true;
          return bbFetchAppCss().then(function (css) {
            self._snapshot = bbBuildSnapshot(self.redact, css);
            return self.capture(css);
          }).finally(function () { self.capturing = false; });
        },

        // capture renders the current viewport to a JPEG data URL. Redaction
        // (when on) and CSS inlining both run in html2canvas's onclone hook so
        // the live page is never mutated. Best-effort: any failure leaves
        // screenshot empty.
        capture: function (css) {
          var self = this;
          var redact = this.redact;
          return this.loadHtml2Canvas().then(function (h2c) {
            if (!h2c) {
              self.captureNote = 'Screenshot library unavailable — filing without an image.';
              self.screenshot = '';
              return;
            }
            var bg = '';
            try { bg = getComputedStyle(document.body).backgroundColor; } catch (e) { bg = ''; }
            return h2c(document.documentElement, {
              backgroundColor: bg || '#ffffff',
              useCORS: true,
              allowTaint: false,
              logging: false,
              scale: Math.min(Math.max(window.devicePixelRatio || 1, 2), 3),
              // Capture exactly the on-screen viewport at the user's current
              // scroll. windowWidth/Height MUST be the viewport size, not the
              // full document size: passing scrollWidth/scrollHeight makes
              // html2canvas lay the clone out as a full-height window scrolled
              // to the top, so the (x,y) crop never reaches the visible region
              // and every capture comes out scrolled to the top of the page.
              x: window.scrollX,
              y: window.scrollY,
              width: window.innerWidth,
              height: window.innerHeight,
              windowWidth: window.innerWidth,
              windowHeight: window.innerHeight,
              ignoreElements: function (el) { return el.id === 'bb-dev-reporter'; },
              onclone: function (clonedDoc) {
                if (redact && clonedDoc && clonedDoc.body) {
                  try { bbRedactClone(clonedDoc.body); } catch (e) {}
                }
                // Inline the app CSS so styling is deterministic — the clone
                // iframe otherwise re-fetches the external stylesheet and can
                // rasterize before it applies, yielding an unstyled capture.
                bbInlineCss(clonedDoc.documentElement || clonedDoc, css);
              },
            }).then(function (canvas) {
              self.screenshot = self.canvasToJpeg(canvas, 2560, 0.92);
            });
          }).catch(function () {
            self.captureNote = 'Could not capture a screenshot — filing without one.';
            self.screenshot = '';
          });
        },

        // canvasToJpeg downscales only when the capture is wider than maxW (a
        // guard against 3×+ displays), then encodes at the given JPEG quality.
        // The artifact store accepts up to 25 MB and we capture the viewport
        // only, so a single high-quality encode stays well within limits — no
        // need for the old aggressive shrink-to-1 MB loop.
        canvasToJpeg: function (canvas, maxW, quality) {
          var c = canvas;
          if (canvas.width > maxW) {
            var ratio = maxW / canvas.width;
            var out = document.createElement('canvas');
            out.width = maxW;
            out.height = Math.round(canvas.height * ratio);
            out.getContext('2d').drawImage(canvas, 0, 0, out.width, out.height);
            c = out;
          }
          return c.toDataURL('image/jpeg', quality);
        },

        // Loads html2canvas-pro — a maintained fork of html2canvas that
        // supports modern CSS color functions (oklch, color-mix), which
        // Breadbox's theme uses everywhere; the original 1.4.1 throws on them.
        // It's a drop-in: the UMD build still exposes window.html2canvas.
        loadHtml2Canvas: function () {
          if (window.html2canvas) return Promise.resolve(window.html2canvas);
          if (this._h2cPromise) return this._h2cPromise;
          this._h2cPromise = new Promise(function (resolve) {
            var s = document.createElement('script');
            s.src = 'https://cdn.jsdelivr.net/npm/html2canvas-pro@2.0.4/dist/html2canvas-pro.min.js';
            s.crossOrigin = 'anonymous';
            s.onload = function () { resolve(window.html2canvas || null); };
            s.onerror = function () { resolve(null); };
            document.head.appendChild(s);
          });
          return this._h2cPromise;
        },

        // pageURL returns the URL to report. When redacting, the query string +
        // hash are dropped (they can carry search terms like a merchant name).
        pageURL: function () {
          return this.redact ? (location.origin + location.pathname) : location.href;
        },

        metadata: function () {
          var theme = document.documentElement.getAttribute('data-theme');
          if (!theme) {
            theme = (window.matchMedia && matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
          }
          return {
            viewport: window.innerWidth + '×' + window.innerHeight,
            device_pixel_ratio: window.devicePixelRatio || 1,
            screen: (window.screen ? window.screen.width + '×' + window.screen.height : ''),
            theme: theme,
            user_agent: navigator.userAgent,
            language: navigator.language || '',
            referrer: this.redact ? '' : (document.referrer || ''),
            app_version: this.version,
            current_page: this.page,
            reported_by: this.user,
            client_time: new Date().toISOString(),
            redacted: this.redact,
          };
        },

        submit: function () {
          if (!this.title.trim() || this.submitting) return;
          this.submitting = true;
          this.error = '';
          var self = this;
          var payload = {
            type: this.type,
            title: this.title.trim(),
            description: this.description,
            page_url: this.pageURL(),
            page_path: location.pathname,
            screenshot: this.includeScreenshot ? this.screenshot : '',
            html: this._snapshot,
            metadata: this.metadata(),
          };
          fetch('/-/dev-reports', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
          }).then(function (resp) {
            return resp.json().catch(function () { return {}; }).then(function (data) {
              return { ok: resp.ok, data: data };
            });
          }).then(function (r) {
            self.submitting = false;
            if (!r.ok) {
              self.error = (r.data && r.data.error && r.data.error.message) || 'Failed to file the report.';
              return;
            }
            var data = r.data || {};
            if (data.status === 'open' && data.github_issue_url) {
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: {
                message: 'Issue #' + data.github_issue_number + ' filed',
                type: 'success',
                href: data.github_issue_url,
                linkLabel: 'Open issue',
                duration: 6000,
              } }));
            } else if (data.status === 'draft' && data.draft_url) {
              // Open the prefilled GitHub draft. window.open can be blocked
              // after an async fetch, so always surface a toast link too.
              try { window.open(data.draft_url, '_blank', 'noopener'); } catch (e) {}
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: {
                message: 'GitHub draft ready — review & submit',
                type: 'info',
                href: data.draft_url,
                linkLabel: 'Open draft',
                duration: 8000,
              } }));
            } else if (data.status === 'saved') {
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: {
                message: 'Report saved to Breadbox (GitHub not configured)',
                type: 'info',
                duration: 5000,
              } }));
            } else if (data.status === 'failed') {
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: {
                message: 'Saved locally — GitHub filing failed',
                type: 'warning',
                duration: 6000,
              } }));
            } else {
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: {
                message: 'Report saved',
                type: 'success',
              } }));
            }
            self.panelOpen = false;
            self.reset();
          }).catch(function () {
            self.submitting = false;
            self.error = 'Network error — please try again.';
          });
        },
      };
    });
  });
})();
