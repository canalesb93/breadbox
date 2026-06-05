// cronField — Alpine factory for the shared CronField templ component.
//
// Reads its config (initial cron value, preset catalog, preview endpoint, the
// custom-preset key) from the root element's data-config JSON. Renders preset
// chips, a custom-expression input, and a debounced live preview fetched from
// the server (description + next fire times, in the instance timezone).
//
// The active preset is derived from the cron VALUE on init (presetFromCron), so
// an expression with no matching preset shows "Custom" with the raw cron — the
// fix for the old form preselecting a wrong preset (e.g. "Every 15 minutes" for
// "0 */12 * * *").
document.addEventListener('alpine:init', function () {
  Alpine.data('cronField', function () {
    return {
      presets: [],
      cron: '',
      active: 'custom',
      customKey: 'custom',
      previewUrl: '/-/cron/preview',
      preview: { valid: false, description: '', runs: [], tz: '' },
      _debounce: null,

      init: function () {
        var cfg = {};
        try { cfg = JSON.parse(this.$el.dataset.config || '{}'); } catch (e) { cfg = {}; }
        this.presets = cfg.presets || [];
        this.previewUrl = cfg.previewUrl || '/-/cron/preview';
        this.customKey = cfg.customKey || 'custom';
        this.cron = (cfg.value || '').trim();
        this.active = this.presetFromCron(this.cron);
        if (this.cron) this.fetchPreview();
      },

      // presetFromCron returns the preset key whose cron matches the expression
      // exactly, or the custom key when none match.
      presetFromCron: function (expr) {
        expr = (expr || '').trim();
        if (!expr) return this.customKey;
        for (var i = 0; i < this.presets.length; i++) {
          if (this.presets[i].cron && this.presets[i].cron === expr) {
            return this.presets[i].key;
          }
        }
        return this.customKey;
      },

      applyPreset: function (preset) {
        if (preset.key === this.customKey) {
          this.active = this.customKey;
          this.refresh();
          return;
        }
        this.cron = preset.cron;
        this.active = preset.key;
        this.refresh();
      },

      onInput: function () {
        this.active = this.customKey;
        this.refresh();
      },

      refresh: function () {
        var self = this;
        if (this._debounce) clearTimeout(this._debounce);
        this._debounce = setTimeout(function () { self.fetchPreview(); }, 250);
      },

      fetchPreview: function () {
        var self = this;
        var expr = (this.cron || '').trim();
        if (!expr) {
          self.preview = { valid: false, description: '', runs: [], tz: '' };
          return;
        }
        fetch(this.previewUrl + '?cron=' + encodeURIComponent(expr), {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) { return res.json(); })
          .then(function (data) {
            self.preview = {
              valid: !!(data && data.valid),
              description: (data && data.description) || '',
              runs: (data && data.next_runs) || [],
              tz: (data && data.tz_label) || '',
            };
          })
          .catch(function () {
            self.preview = { valid: false, description: 'Preview unavailable', runs: [], tz: '' };
          });
      },
    };
  });
});
