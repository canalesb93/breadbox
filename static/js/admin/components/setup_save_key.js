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

    // Copy text to the clipboard with a fallback for insecure contexts.
    // navigator.clipboard only exists on HTTPS or localhost — a Breadbox
    // reached over a plain-HTTP LAN IP (http://192.168.x.x:8080) has no
    // clipboard API, so the modern path silently no-ops there. Fall back
    // to a hidden-textarea + execCommand('copy'), which still works over
    // plain HTTP. Returns true on success.
    function copyToClipboard(text) {
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).catch(function () { legacyCopy(text); });
        return true;
      }
      return legacyCopy(text);
    }

    function legacyCopy(text) {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      // Keep it out of view and off the layout, but still selectable.
      ta.style.position = 'fixed';
      ta.style.top = '-9999px';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      ta.setSelectionRange(0, text.length);
      var ok = false;
      try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
      document.body.removeChild(ta);
      return ok;
    }

    // Copy.
    var copyBtn = root.querySelector('[data-action="copy"]');
    var copyResetTimer = null;
    if (copyBtn) {
      copyBtn.addEventListener('click', function () {
        var ok = copyToClipboard(key);
        var ic = copyBtn.querySelector('[data-copy-icon]');
        var done = copyBtn.querySelector('[data-copy-done]');
        var lbl = copyBtn.querySelector('[data-copy-label]');
        if (!ok) {
          // Last-resort graceful degradation: reveal the key and select it
          // so the user can copy it by hand (the original complaint).
          setRevealed(true);
          selectKeyText();
          if (lbl) lbl.textContent = 'Press ⌘/Ctrl+C to copy';
          return;
        }
        copyBtn.classList.add('btn-success');
        copyBtn.classList.remove('btn-primary', 'btn-soft');
        if (ic) ic.classList.add('hidden');
        if (done) done.classList.remove('hidden');
        if (lbl) lbl.textContent = 'Copied';
        if (copyResetTimer) clearTimeout(copyResetTimer);
        copyResetTimer = setTimeout(function () {
          copyBtn.classList.remove('btn-success');
          copyBtn.classList.add('btn-primary', 'btn-soft');
          if (ic) ic.classList.remove('hidden');
          if (done) done.classList.add('hidden');
          if (lbl) lbl.textContent = 'Copy to clipboard';
        }, 2000);
      });
    }

    // Select the visible key text so a manual copy picks up exactly the
    // 64-char value (used by the copy fallback path).
    function selectKeyText() {
      try {
        var range = document.createRange();
        range.selectNodeContents(keyText);
        var sel = window.getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
      } catch (e) { /* selection is best-effort */ }
    }
  }

  onReady(init);
})();
