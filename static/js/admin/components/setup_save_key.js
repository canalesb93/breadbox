// Page behavior for /setup/save-key — reveal toggle, clipboard copy,
// and ".env" download for the one-time encryption-key reveal screen.
//
// Plain vanilla JS (not Alpine): the wizard layout deliberately doesn't
// load Alpine.js. The encryption key is read from a `data-key-value`
// attribute on the display container rather than an <input>, so we can
// render it as wrap-friendly mono text instead of a single-line field
// that truncates a 64-char hex string.
(function () {
  function onReady(fn) {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fn);
    } else {
      fn();
    }
  }

  function init() {
    var root = document.querySelector('[data-setup-save-key]');
    if (!root) return;
    var display = root.querySelector('[data-key-display]');
    if (!display) return;
    var keyText = display.querySelector('[data-key-text]');
    if (!keyText) return;

    var key = display.getAttribute('data-key-value') || '';
    if (!key) return;

    var masked = '●'.repeat(key.length);
    var revealed = false;

    var toggleBtn = root.querySelector('[data-action="toggle-reveal"]');
    var toggleLabel = toggleBtn && toggleBtn.querySelector('[data-toggle-label]');
    var eyeIcon = toggleBtn && toggleBtn.querySelector('[data-eye]');
    var eyeOffIcon = toggleBtn && toggleBtn.querySelector('[data-eye-off]');

    function setRevealed(next) {
      revealed = !!next;
      keyText.textContent = revealed ? key : masked;
      // Only enable user-select-all once the real key is visible —
      // selecting a column of bullets isn't useful.
      display.classList.toggle('select-all', revealed);
      display.setAttribute('aria-label', revealed ? 'Encryption key (revealed)' : 'Encryption key (hidden)');
      if (toggleBtn) {
        toggleBtn.setAttribute('aria-pressed', String(revealed));
        if (toggleLabel) toggleLabel.textContent = revealed ? 'Hide' : 'Show';
        if (eyeIcon) eyeIcon.classList.toggle('hidden', revealed);
        if (eyeOffIcon) eyeOffIcon.classList.toggle('hidden', !revealed);
      }
    }
    // SSR rendered the masked form already; just sync state without
    // touching textContent so screen readers don't re-announce.
    setRevealed(false);

    if (toggleBtn) {
      toggleBtn.addEventListener('click', function () { setRevealed(!revealed); });
    }

    // Copy.
    var copyBtn = root.querySelector('[data-action="copy"]');
    var copyResetTimer = null;
    if (copyBtn) {
      copyBtn.addEventListener('click', function () {
        if (!navigator.clipboard) return;
        navigator.clipboard.writeText(key).then(function () {
          copyBtn.classList.add('btn-success');
          copyBtn.classList.remove('btn-primary', 'btn-soft');
          var ic = copyBtn.querySelector('[data-copy-icon]');
          var ok = copyBtn.querySelector('[data-copy-done]');
          var lbl = copyBtn.querySelector('[data-copy-label]');
          if (ic) ic.classList.add('hidden');
          if (ok) ok.classList.remove('hidden');
          if (lbl) lbl.textContent = 'Copied to clipboard';
          if (copyResetTimer) clearTimeout(copyResetTimer);
          copyResetTimer = setTimeout(function () {
            copyBtn.classList.remove('btn-success');
            copyBtn.classList.add('btn-primary', 'btn-soft');
            if (ic) ic.classList.remove('hidden');
            if (ok) ok.classList.add('hidden');
            if (lbl) lbl.textContent = 'Copy to clipboard';
          }, 2000);
        });
      });
    }

    // Download .env.
    var dlBtn = root.querySelector('[data-action="download"]');
    var dlResetTimer = null;
    if (dlBtn) {
      dlBtn.addEventListener('click', function () {
        var contents = '# Breadbox encryption key -- store this somewhere safe.\n'
          + '# Without it, encrypted bank credentials cannot be decrypted.\n'
          + 'ENCRYPTION_KEY=' + key + '\n';
        var blob = new Blob([contents], { type: 'text/plain;charset=utf-8' });
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url;
        a.download = 'breadbox.env';
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
        var dlIcon = dlBtn.querySelector('[data-download-icon]');
        var dlDone = dlBtn.querySelector('[data-download-done]');
        var dlLabel = dlBtn.querySelector('[data-download-label]');
        if (dlIcon) dlIcon.classList.add('hidden');
        if (dlDone) dlDone.classList.remove('hidden');
        if (dlLabel) dlLabel.textContent = 'Downloaded';
        if (dlResetTimer) clearTimeout(dlResetTimer);
        dlResetTimer = setTimeout(function () {
          if (dlIcon) dlIcon.classList.remove('hidden');
          if (dlDone) dlDone.classList.add('hidden');
          if (dlLabel) dlLabel.textContent = 'Download .env';
        }, 2000);
      });
    }
  }

  onReady(init);
})();
