// Global prompt modal — a single dialog mounted in base.html, opened from
// anywhere via a window CustomEvent:
//
//   $dispatch('bb-prompt-modal', {
//     mode: 'preview' | 'editable',   // default 'preview'
//     edit: true,                      // editable: start in the editor (else preview)
//     title: 'Custom workflow prompt',
//     subtitle: 'Markdown — runs verbatim on every run.',
//     value: '# the markdown',         // initial text (else read from target)
//     target: '#prompt-field' | el,    // editable: where Save writes the value
//     onSaved: (v) => {},              // editable: optional callback after save
//     saveLabel: 'Save prompt',
//   })
//
// Two modes:
//   - preview  : read-only render of `value` + Copy + Close.
//   - editable : Edit/Preview toggle, ⌘/Ctrl+Enter (or Save) writes `value`
//                back into `target` (and/or calls onSaved), then closes.
//
// The preview renders server-side (POST /-/markdown/preview → goldmark +
// bluemonday) so there is no client-side Markdown parser; CSRF is injected by
// the global fetch wrapper in base.html.
document.addEventListener('alpine:init', function () {
  Alpine.data('bbPromptModal', function () {
    return {
      mode: 'preview',
      view: 'preview', // 'edit' | 'preview'
      title: '',
      subtitle: '',
      value: '',
      loading: false,
      copied: false,
      saveLabel: 'Save',
      _targetEl: null,
      _onSaved: null,
      _seq: 0, // guards against out-of-order render responses

      open: function (detail) {
        detail = detail || {};
        this.mode = detail.mode === 'editable' ? 'editable' : 'preview';
        this.title = detail.title || 'Prompt';
        this.subtitle = detail.subtitle || '';
        this.saveLabel = detail.saveLabel || 'Save';
        this.copied = false;

        // Resolve the target field first so we can fall back to its value.
        this._targetEl = null;
        if (detail.target) {
          this._targetEl =
            typeof detail.target === 'string'
              ? document.querySelector(detail.target)
              : detail.target;
        }
        if (detail.value != null) {
          this.value = String(detail.value);
        } else if (this._targetEl && 'value' in this._targetEl) {
          this.value = this._targetEl.value || '';
        } else {
          this.value = '';
        }
        this._onSaved = typeof detail.onSaved === 'function' ? detail.onSaved : null;

        // Editable opens in the editor when asked, else in preview; preview-only
        // is always the rendered view.
        this.view = this.mode === 'editable' && detail.edit ? 'edit' : 'preview';

        var self = this;
        this.$nextTick(function () {
          // $root is the component root (the <dialog>); $el would be whatever
          // element invoked the handler.
          if (self.$root && self.$root.showModal && !self.$root.open) self.$root.showModal();
          if (self.view === 'preview') self.renderPreview();
          else self.focusEditor();
        });
      },

      close: function () {
        if (this.$root && this.$root.close && this.$root.open) this.$root.close();
      },

      setView: function (v) {
        if (this.view === v) return;
        this.view = v;
        if (v === 'preview') this.renderPreview();
        else this.focusEditor();
      },

      focusEditor: function () {
        var self = this;
        this.$nextTick(function () {
          var t = self.$refs.editor;
          if (t) t.focus();
        });
      },

      // Render the current buffer to sanitized HTML server-side and inject it.
      renderPreview: function () {
        var self = this;
        var seq = ++self._seq;
        self.loading = true;
        fetch('/-/markdown/preview', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ text: self.value || '' }),
        })
          .then(function (r) {
            return r.ok ? r.json() : Promise.reject(r.status);
          })
          .then(function (data) {
            if (seq !== self._seq) return; // a newer render superseded this one
            self.loading = false;
            self.$nextTick(function () {
              var el = self.$refs.previewBody;
              if (!el) return;
              el.innerHTML = (data && data.html) || '';
              // Render <i data-lucide> placeholders the markdown emitted
              // (callout / code-copy icons).
              if (window.lucide && typeof window.lucide.createIcons === 'function') {
                window.lucide.createIcons();
              }
              el.scrollTop = 0;
            });
          })
          .catch(function () {
            if (seq !== self._seq) return;
            self.loading = false;
            self.$nextTick(function () {
              var el = self.$refs.previewBody;
              if (el) el.innerHTML = '<p>Could not render the preview. Please try again.</p>';
            });
          });
      },

      // Copy the raw Markdown source (not the rendered HTML).
      copy: function () {
        var self = this;
        var text = self.value || '';
        if (!text) return;
        var done = function () {
          self.copied = true;
          setTimeout(function () {
            self.copied = false;
          }, 1500);
        };
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(done).catch(function () {
            self._copyFallback(text, done);
          });
        } else {
          self._copyFallback(text, done);
        }
      },

      _copyFallback: function (text, done) {
        try {
          var ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.opacity = '0';
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
          done();
        } catch (e) {
          console.error('prompt-modal copy fallback failed', e);
        }
      },

      // Write the buffer back to the originating field and/or fire the
      // callback, then close. No-op in preview-only mode.
      save: function () {
        if (this.mode !== 'editable') return;
        var v = this.value || '';
        if (this._targetEl && 'value' in this._targetEl) {
          this._targetEl.value = v;
          // Let Alpine x-model / form listeners on the target react.
          this._targetEl.dispatchEvent(new Event('input', { bubbles: true }));
          this._targetEl.dispatchEvent(new Event('change', { bubbles: true }));
        }
        if (this._onSaved) {
          try {
            this._onSaved(v);
          } catch (e) {
            console.error('prompt-modal onSaved failed', e);
          }
        }
        this.close();
      },
    };
  });
});
