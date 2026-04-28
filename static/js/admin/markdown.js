// Shared markdown rendering for any element carrying `data-markdown="..."`.
//
// Used by:
//   - /reports/{id}              -> .bb-report-body (full-page markdown)
//   - /transactions/{id}         -> .bb-comment-bubble (annotation comments)
//
// Loaded as a sibling of marked + DOMPurify CDN scripts. Auto-runs on
// DOMContentLoaded and exposes `window.bbRenderMarkdown(root)` so pages
// that mutate the DOM after load (e.g. transaction_detail.js inserting
// freshly-fetched timeline rows) can re-process newly-inserted nodes
// without a full reload.
//
// Per-element opt-ins via data-* attributes:
//   data-markdown-breaks="true"  -> marked `breaks: true` (single newlines
//                                    become <br>) — friendlier for chat-style
//                                    user comments.
//
// Behavior applied to every rendered block:
//   - DOMPurify.sanitize() guards against XSS in untrusted input.
//   - External links get target="_blank" + rel="noopener" so leaving the
//     admin doesn't lose the user's spot.
//   - <table> elements are wrapped in .bb-report-table-wrap for horizontal
//     scrolling on narrow viewports.
//   - The last block-level child gets `!mb-0` so the container's bottom
//     padding stays even (no double gap from a trailing <p class="mb-3">).
//
// Idempotent: an element with `data-markdown-rendered="1"` is skipped on
// subsequent passes, so calling `bbRenderMarkdown(document)` multiple
// times after partial updates is safe.
(function () {
  function renderOne(el) {
    if (!el || el.dataset.markdownRendered === '1') return;
    var md = el.getAttribute('data-markdown');
    if (md === null) return;
    if (typeof marked === 'undefined' || typeof DOMPurify === 'undefined') return;

    var opts = {};
    if (el.dataset.markdownBreaks === 'true') opts.breaks = true;

    var html;
    try {
      html = DOMPurify.sanitize(marked.parse(md, opts));
    } catch (e) {
      // Fall back to a pre-wrapped escaped block so the user still sees
      // their content even if marked/DOMPurify barf on edge-case input.
      html = '<pre style="white-space:pre-wrap">' +
        md.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') +
        '</pre>';
    }
    el.innerHTML = html;
    el.dataset.markdownRendered = '1';

    el.querySelectorAll('a').forEach(function (a) {
      var href = a.getAttribute('href') || '';
      if (a.href && !a.href.startsWith(window.location.origin) && !href.startsWith('/')) {
        a.setAttribute('target', '_blank');
        a.setAttribute('rel', 'noopener');
      }
    });

    el.querySelectorAll('table').forEach(function (t) {
      if (t.parentNode && t.parentNode.classList && t.parentNode.classList.contains('bb-report-table-wrap')) return;
      var wrap = document.createElement('div');
      wrap.className = 'bb-report-table-wrap';
      t.parentNode.insertBefore(wrap, t);
      wrap.appendChild(t);
    });

    var lastChild = el.lastElementChild;
    if (lastChild) lastChild.classList.add('!mb-0');
  }

  function renderAll(root) {
    var scope = root || document;
    // Include the root itself when it carries data-markdown.
    if (scope.nodeType === 1 && scope.hasAttribute && scope.hasAttribute('data-markdown')) {
      renderOne(scope);
    }
    if (typeof scope.querySelectorAll !== 'function') return;
    scope.querySelectorAll('[data-markdown]').forEach(renderOne);
  }

  window.bbRenderMarkdown = renderAll;

  document.addEventListener('DOMContentLoaded', function () { renderAll(document); });
})();
