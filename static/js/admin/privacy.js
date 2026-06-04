// Privacy mode — client-side presentation layer for sensitive financial data.
//
// Three modes, persisted per-browser in localStorage('bb-privacy'):
//   'real'      — actual data (default).
//   'obfuscate' — same-shape but obviously-fake "matrix/hex" glitch
//                 ($12,456.00 → $3a,4d4.0f, Starbucks → Xq7v8bn2). The real
//                 text is replaced in the DOM; deterministic per value so it's
//                 stable across renders (no shimmer).
//   'hide'      — blur the real value via CSS (see input.css). No JS rewrite.
//
// Sensitive nodes are marked server-side with `data-private="<kind>"`
// (amount | merchant | account | institution | mask). The kind is advisory —
// one glitch function preserves each character class, so every kind is handled
// uniformly today; the attribute keeps the door open for per-kind strategies.
//
// THREAT MODEL: presentation-only. Real values still exist in the DOM /
// devtools / network — this defends against shoulder-surfing, screenshots, and
// screen-shares, NOT against someone with browser access. Mirrors the
// "privacy mode" in YNAB/Copilot/Mint.
//
// Loaded non-deferred in <head> (base.html) so window.bbPrivacy exists before
// Alpine's `privacy` store initializes. The matching pre-paint IIFE in
// base.html sets `data-privacy` + the `privacy-pending` blur class before first
// paint, so obfuscate never flashes real values before this script rewrites
// them.
(function () {
  'use strict';

  var KEY = 'bb-privacy';
  var MODES = { real: 1, obfuscate: 1, hide: 1 };
  var DONE = 'data-bb-glitched'; // marks an element whose text we've replaced

  function readMode() {
    try {
      var v = localStorage.getItem(KEY);
      if (v && MODES[v]) return v;
    } catch (_) { /* private-mode Safari */ }
    return 'real';
  }

  var state = readMode();

  // ----- deterministic glitch -----

  // FNV-1a-ish 32-bit hash, seeds the per-value glitch so a given input always
  // maps to the same output.
  function hash32(str) {
    var h = 0x811c9dc5 >>> 0;
    for (var i = 0; i < str.length; i++) {
      h ^= str.charCodeAt(i);
      h = Math.imul(h, 0x01000193) >>> 0;
    }
    return h >>> 0;
  }

  var HEX = '0123456789abcdef';
  var LOWER = 'abcdefghijklmnopqrstuvwxyz';
  var UPPER = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';

  // Glitch a string in place: digits → hex (the "matrix" read), letters →
  // scrambled same-case letters, everything else (spaces, $ , . - · / etc.)
  // verbatim. Length, grouping, currency symbols, and word boundaries survive,
  // so layout and tabular-nums alignment hold.
  function glitch(text) {
    if (!text) return text;
    var seed = hash32(text);
    var out = '';
    for (var i = 0; i < text.length; i++) {
      var c = text.charCodeAt(i);
      // advance an LCG per position so adjacent same-class chars differ
      seed = (Math.imul(seed, 1664525) + 1013904223 + i) >>> 0;
      if (c >= 48 && c <= 57) {            // 0-9
        out += HEX[seed % 16];
      } else if (c >= 97 && c <= 122) {    // a-z
        out += LOWER[seed % 26];
      } else if (c >= 65 && c <= 90) {     // A-Z
        out += UPPER[seed % 26];
      } else {
        out += text[i];                    // punctuation / whitespace verbatim
      }
    }
    return out;
  }

  // ----- DOM application -----

  // Replace every non-whitespace text node under `el` with its glitch. Only
  // text nodes are touched, so child elements (category dots, icons, badges)
  // inside a marked container survive untouched. The original is stashed on the
  // node so restore() is exact.
  function obfuscateEl(el) {
    if (el.getAttribute(DONE) === '1') return;
    var walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT, null);
    var n;
    while ((n = walker.nextNode())) {
      if (!n.nodeValue || !n.nodeValue.trim()) continue;
      if (n.__bbReal == null) n.__bbReal = n.nodeValue;
      n.nodeValue = glitch(n.__bbReal);
    }
    el.setAttribute(DONE, '1');
  }

  function restoreEl(el) {
    if (el.getAttribute(DONE) !== '1') return;
    var walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT, null);
    var n;
    while ((n = walker.nextNode())) {
      if (n.__bbReal != null) n.nodeValue = n.__bbReal;
    }
    el.removeAttribute(DONE);
  }

  // Apply the current mode to all marked nodes under `root`. obfuscate rewrites
  // text; real/hide restore the real text (hide blurs it via CSS).
  function applyAll(root) {
    root = root || document.body;
    if (!root || !root.querySelectorAll) return;
    var fn = state === 'obfuscate' ? obfuscateEl : restoreEl;
    if (root.nodeType === 1 && root.matches && root.matches('[data-private]')) fn(root);
    var els = root.querySelectorAll('[data-private]');
    for (var i = 0; i < els.length; i++) fn(els[i]);
  }

  // ----- observer: catch dynamically-inserted nodes (fetch+innerHTML swaps,
  // Alpine re-renders). Only does work in obfuscate; hide is pure CSS and real
  // needs nothing. Our own text-node edits are characterData mutations, which
  // we don't observe, so this never re-triggers on itself.
  var observer = null;
  function startObserver() {
    if (observer) return;
    observer = new MutationObserver(function (muts) {
      if (state !== 'obfuscate') return;
      for (var i = 0; i < muts.length; i++) {
        var added = muts[i].addedNodes;
        for (var j = 0; j < added.length; j++) {
          var node = added[j];
          if (node.nodeType !== 1) continue;
          applyAll(node);
        }
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  // ----- public API -----

  function setMode(mode) {
    if (!MODES[mode]) return;
    state = mode;
    try { localStorage.setItem(KEY, mode); } catch (_) {}
    var root = document.documentElement;
    if (mode === 'real') root.removeAttribute('data-privacy');
    else root.setAttribute('data-privacy', mode);
    applyAll(document.body);
    root.classList.remove('privacy-pending');
  }

  window.bbPrivacy = {
    get mode() { return state; },
    set: setMode,
    cycle: function () {
      var order = { real: 'obfuscate', obfuscate: 'hide', hide: 'real' };
      setMode(order[state] || 'real');
    },
  };

  // ----- init: run the first pass once the body exists, then reveal -----
  function init() {
    var root = document.documentElement;
    if (state === 'real') root.removeAttribute('data-privacy');
    else root.setAttribute('data-privacy', state);
    applyAll(document.body);
    root.classList.remove('privacy-pending'); // reveal fakes / drop the gap blur
    startObserver();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
