// Page behavior for /setup/save-key — reveal toggle, clipboard copy,
// and ".env" download for the one-time encryption-key reveal screen.
//
// Plain vanilla JS (not Alpine): the wizard layout deliberately doesn't
// load Alpine.js. Reads the key from the readonly <input> rather than a
// JSONScript blob — the key is already in the DOM and there's no
// concern about HTML escaping since it's a 64-char hex string.
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

    var input = root.querySelector('#encryption-key');
    if (!input) return;

    var key = input.value;
    var filenameBase = root.getAttribute('data-filename') || 'breadbox';
    var filename = filenameBase.replace(/[^A-Za-z0-9._-]+/g, '-') + '.env';

    // Reveal toggle.
    var toggleBtn = root.querySelector('[data-action="toggle-reveal"]');
    if (toggleBtn) {
      toggleBtn.addEventListener('click', function () {
        var hidden = input.type === 'password';
        input.type = hidden ? 'text' : 'password';
        toggleBtn.setAttribute('aria-label', hidden ? 'Hide encryption key' : 'Reveal encryption key');
        var eye = toggleBtn.querySelector('[data-eye]');
        var eyeOff = toggleBtn.querySelector('[data-eye-off]');
        if (eye) eye.classList.toggle('hidden', hidden);
        if (eyeOff) eyeOff.classList.toggle('hidden', !hidden);
      });
    }

    // Copy.
    var copyBtn = root.querySelector('[data-action="copy"]');
    if (copyBtn) {
      copyBtn.addEventListener('click', function () {
        if (!navigator.clipboard) return;
        navigator.clipboard.writeText(key).then(function () {
          copyBtn.classList.add('!btn-success');
          var ic = copyBtn.querySelector('[data-copy-icon]');
          var ok = copyBtn.querySelector('[data-copy-done]');
          var lbl = copyBtn.querySelector('[data-copy-label]');
          if (ic) ic.classList.add('hidden');
          if (ok) ok.classList.remove('hidden');
          if (lbl) lbl.textContent = 'Copied!';
          setTimeout(function () {
            copyBtn.classList.remove('!btn-success');
            if (ic) ic.classList.remove('hidden');
            if (ok) ok.classList.add('hidden');
            if (lbl) lbl.textContent = 'Copy';
          }, 2000);
        });
      });
    }

    // Download .env file.
    var dlBtn = root.querySelector('[data-action="download"]');
    if (dlBtn) {
      dlBtn.addEventListener('click', function () {
        var contents = '# Breadbox encryption key -- store this somewhere safe.\n'
          + '# Without it, encrypted bank credentials cannot be decrypted.\n'
          + 'ENCRYPTION_KEY=' + key + '\n';
        var blob = new Blob([contents], { type: 'text/plain;charset=utf-8' });
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
      });
    }
  }

  onReady(init);
})();
