// Copy-to-clipboard for server-rendered markdown code blocks.
//
// internal/markdown wraps every fenced code block in
// <div class="bb-code"> with a header bar containing a
// <button data-bb-copy>. This delegated listener copies the block's text.
// One global listener covers every .bb-code on the page (and any injected
// later via the agent-run live poll or the transaction timeline), so there's
// no per-page wiring and no markdown parser on the client.
(function () {
  function flash(btn) {
    var label = btn.querySelector('.bb-code-copy-label');
    var prev = label ? label.textContent : '';
    btn.classList.add('is-copied');
    if (label) label.textContent = 'Copied';
    setTimeout(function () {
      btn.classList.remove('is-copied');
      if (label) label.textContent = prev || 'Copy';
    }, 1400);
  }

  function copyText(text, btn) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function () { flash(btn); }, function () {});
      return;
    }
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.top = '-9999px';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); flash(btn); } catch (e) { /* ignore */ }
    document.body.removeChild(ta);
  }

  document.addEventListener('click', function (e) {
    if (!e.target.closest) return;
    var btn = e.target.closest('[data-bb-copy]');
    if (!btn) return;
    var wrap = btn.closest('.bb-code');
    var pre = wrap && wrap.querySelector('pre');
    if (!pre) return;
    copyText(pre.innerText.replace(/\n$/, ''), btn);
  });
})();
