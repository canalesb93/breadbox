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
// TRANSITIONS: switching to/from obfuscate runs a brief "decode" scramble —
// each marked value flickers through random same-class glyphs and locks in
// left-to-right (the matrix read). Only on-screen nodes animate (off-screen
// ones snap), it respects prefers-reduced-motion, and a token cancels an
// in-flight run so rapid toggles don't pile up. The hide blur eases via CSS.
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

  var REDUCED = false;
  try { REDUCED = window.matchMedia('(prefers-reduced-motion: reduce)').matches; } catch (_) {}

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

  // Is this char glyph an obfuscatable class (digit / letter)? Punctuation and
  // whitespace are left alone, both by glitch() and the decode scramble.
  function isGlyph(code) {
    return (code >= 48 && code <= 57) || (code >= 97 && code <= 122) || (code >= 65 && code <= 90);
  }

  // A random same-class glyph for the decode flicker (non-deterministic — the
  // flicker is meant to churn; only the locked, settled value is deterministic).
  function randGlyph(code) {
    if (code >= 48 && code <= 57) return HEX[(Math.random() * 16) | 0];
    if (code >= 97 && code <= 122) return LOWER[(Math.random() * 26) | 0];
    return UPPER[(Math.random() * 26) | 0];
  }

  // Partially-decoded string at progress `prog` (0..1): glyph positions before
  // the lock front show the final char; the rest flicker through random
  // same-class glyphs. Punctuation/spaces are always the final char. `real` is
  // used only for per-position character class (it shares length with target).
  function scramble(real, target, prog) {
    var len = target.length;
    var locked = Math.floor(prog * len + 1e-6);
    var out = '';
    for (var i = 0; i < len; i++) {
      if (i < locked) { out += target.charAt(i); continue; }
      var code = (real.charCodeAt(i) || target.charCodeAt(i));
      out += isGlyph(code) ? randGlyph(code) : target.charAt(i);
    }
    return out;
  }

  // ----- DOM application (instant) -----

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

  // Apply the current mode to all marked nodes under `root`, instantly.
  // obfuscate rewrites text; real/hide restore the real text (hide blurs it via
  // CSS). Used on first paint and for dynamically-inserted nodes.
  function applyAll(root) {
    root = root || document.body;
    if (!root || !root.querySelectorAll) return;
    var fn = state === 'obfuscate' ? obfuscateEl : restoreEl;
    if (root.nodeType === 1 && root.matches && root.matches('[data-private]')) fn(root);
    var els = root.querySelectorAll('[data-private]');
    for (var i = 0; i < els.length; i++) fn(els[i]);
  }

  // ----- DOM application (animated decode) -----

  var animToken = 0; // bumped on every transition so a stale rAF loop bails

  // Animate the text transition between `from` and `to`. The decode scramble
  // only runs when the underlying text actually changes between obfuscated and
  // real (real↔obfuscate); transitions into/out of hide keep the text and let
  // the CSS blur do the work, so we just snap the text and return. Off-screen
  // marked elements snap instantly; only visible ones animate (bounds the
  // per-frame DOM writes to what the user can actually see).
  function animateTo(from, to) {
    var obf = to === 'obfuscate';
    var doScramble = obf || (to === 'real' && from === 'obfuscate');

    var els = document.querySelectorAll('[data-private]');
    var vh = window.innerHeight || document.documentElement.clientHeight || 0;
    var jobs = []; // [{nodes:[{node,real,target}], delay}]

    for (var i = 0; i < els.length; i++) {
      var el = els[i];
      var nodes = [];
      var walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT, null);
      var n;
      while ((n = walker.nextNode())) {
        if (!n.nodeValue || !n.nodeValue.trim()) continue;
        if (n.__bbReal == null) n.__bbReal = n.nodeValue;
        nodes.push({ node: n, real: n.__bbReal, target: obf ? glitch(n.__bbReal) : n.__bbReal });
      }
      // Keep the DONE bookkeeping in lock-step with the instant path so a later
      // applyAll() / observer pass doesn't re-touch these elements.
      if (obf) el.setAttribute(DONE, '1'); else el.removeAttribute(DONE);
      if (!nodes.length) continue;

      var rect = el.getBoundingClientRect();
      var visible = rect.bottom > -40 && rect.top < vh + 40 && rect.width > 0 && rect.height > 0;
      if (!doScramble || !visible || REDUCED) {
        for (var k = 0; k < nodes.length; k++) nodes[k].node.nodeValue = nodes[k].target;
      } else {
        jobs.push({ nodes: nodes, delay: Math.random() * 130 });
      }
    }

    animToken++;
    if (!jobs.length) return;
    var myToken = animToken;
    var DURATION = 320; // per-node decode window (after its stagger delay)
    var start = null;

    function tick(ts) {
      if (myToken !== animToken) return; // superseded by a newer transition
      if (start == null) start = ts;
      var elapsed = ts - start;
      var allDone = true;
      for (var a = 0; a < jobs.length; a++) {
        var job = jobs[a];
        var p = (elapsed - job.delay) / DURATION;
        if (p < 0) { allDone = false; continue; }
        if (p >= 1) p = 1; else allDone = false;
        var eased = 1 - Math.pow(1 - p, 3); // easeOutCubic — quick, soft settle
        var ns = job.nodes;
        for (var b = 0; b < ns.length; b++) {
          var it = ns[b];
          if (it.real === it.target) { it.node.nodeValue = it.target; continue; }
          it.node.nodeValue = p >= 1 ? it.target : scramble(it.real, it.target, eased);
        }
      }
      if (!allDone) requestAnimationFrame(tick);
    }
    requestAnimationFrame(tick);
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

  function setMode(mode, animate) {
    if (!MODES[mode]) return;
    var from = state;
    if (mode === from) return;
    state = mode;
    try { localStorage.setItem(KEY, mode); } catch (_) {}
    var root = document.documentElement;
    if (mode === 'real') root.removeAttribute('data-privacy');
    else root.setAttribute('data-privacy', mode);
    if (animate && !REDUCED) animateTo(from, mode);
    else applyAll(document.body);
    root.classList.remove('privacy-pending');
  }

  window.bbPrivacy = {
    get mode() { return state; },
    // User-initiated changes (avatar menu, command palette, Shift+P) animate;
    // pass animate=false for a silent set.
    set: function (mode) { setMode(mode, true); },
    cycle: function () {
      var order = { real: 'obfuscate', obfuscate: 'hide', hide: 'real' };
      setMode(order[state] || 'real', true);
    },
  };

  // ----- init: run the first pass once the body exists, then reveal -----
  function init() {
    var root = document.documentElement;
    if (state === 'real') root.removeAttribute('data-privacy');
    else root.setAttribute('data-privacy', state);
    applyAll(document.body);             // first paint is instant — no decode
    root.classList.remove('privacy-pending'); // reveal fakes / drop the gap blur
    // Enable the CSS blur transition only AFTER the initial reveal, so the
    // pre-paint blur→content swap on load stays instant (no fade-in flash).
    requestAnimationFrame(function () { root.classList.add('privacy-anim'); });
    startObserver();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
