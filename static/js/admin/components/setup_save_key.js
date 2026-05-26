// Page behavior for /setup/save-key — reveal toggle, clipboard copy,
// and a daisy-styled proxy to the (off-screen) 1Password Save button.
//
// Plain vanilla JS (not Alpine): the wizard layout deliberately doesn't
// load Alpine.js. The encryption key is read from a `data-key-value`
// attribute on the display container rather than an <input>, so we can
// keep the visible content masked at SSR and toggle it client-side
// without ever flashing the plaintext.
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
          if (lbl) lbl.textContent = 'Copied';
          if (copyResetTimer) clearTimeout(copyResetTimer);
          copyResetTimer = setTimeout(function () {
            copyBtn.classList.remove('btn-success');
            copyBtn.classList.add('btn-primary', 'btn-soft');
            if (ic) ic.classList.remove('hidden');
            if (ok) ok.classList.add('hidden');
            if (lbl) lbl.textContent = 'Copy';
          }, 2000);
        });
      });
    }

    // Save in 1Password — forwards the click to the hidden web
    // component's internal button, which is what the 1Password browser
    // extension hooks into. The component lives off-screen so we get to
    // render our own daisy-styled button instead of fighting its
    // shadow-DOM styling. Without the extension, this is a no-op.
    var op1pBtn = root.querySelector('[data-action="save-1password"]');
    if (op1pBtn) {
      op1pBtn.addEventListener('click', function () {
        var opEl = document.querySelector('onepassword-save-button');
        if (!opEl) return;
        var inner = opEl.shadowRoot && opEl.shadowRoot.querySelector('button.onepasswordSaveBtn');
        if (inner) inner.click();
      });
    }
  }

  onReady(init);
})();
