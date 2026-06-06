// cronField — Alpine factory for the shared CronField templ component.
//
// Reads its config (initial cron value, preset catalog, preview endpoint,
// whether presets are viewer-local, the server's UTC offset) from the root
// element's data-config JSON. Renders preset shortcut chips, an always-visible
// custom-expression input, and a debounced one-line live preview fetched from
// the server (a human-readable description of when the schedule next fires).
//
// The active chip is derived from the cron VALUE — an expression with no
// matching preset lights no chip and shows the raw cron in the input, never a
// wrong preset label. The component exposes its `cron` via x-modelable, so a
// caller can two-way bind it (the workflow reconfigure drawer hydrates the
// value after the drawer opens).

document.addEventListener('alpine:init', function () {
  // ── Viewer-local → server-local cron conversion ────────────────────────────
  // Shared with the workflow schedule shortcuts: a chip means a friendly hour in
  // the VIEWER's timezone (e.g. "0 9 * * *" = 9 AM their time); the scheduler
  // fires cron in the SERVER's timezone, so localized chips are shifted by the
  // server↔viewer offset. Mirrors service.shiftCronTimeFields / DescribeCronInTZ.

  function cronSingleInt(s) {
    if (!/^\d+$/.test(String(s).trim())) return null;
    return parseInt(s, 10);
  }

  // Shift a comma-separated day-of-week field by dayDelta days (wrapping 0..6,
  // Sunday normalized to 0). Returns null for non-numeric (ranges / steps).
  function cronShiftDow(field, dayDelta) {
    if (field === '*') return '*';
    var parts = String(field).split(',');
    var out = [];
    for (var i = 0; i < parts.length; i++) {
      var n = cronSingleInt(parts[i]);
      if (n === null) return null;
      if (n < 0 || n > 7) return null;
      if (n === 7) n = 0; // normalize Sunday
      out.push(String(((n + dayDelta) % 7 + 7) % 7));
    }
    return out.join(',');
  }

  // Shift a standard 5-field cron's time-of-day by deltaMin minutes, carrying a
  // midnight wrap into the day-of-week set. Returns null for the
  // non-representable cases (non-integer minute/hour, or a day-of-month
  // constrained schedule whose wrap would land on a different calendar day) so
  // the caller keeps the original. Mirrors service.shiftCronTimeFields.
  function cronShiftTimeFields(expr, deltaMin) {
    var f = String(expr).trim().split(/\s+/);
    if (f.length !== 5) return null;
    var minute = cronSingleInt(f[0]);
    var hour = cronSingleInt(f[1]);
    if (minute === null || hour === null) return null;
    var total = hour * 60 + minute + deltaMin;
    var dayDelta = 0;
    while (total < 0) { total += 1440; dayDelta--; }
    while (total >= 1440) { total -= 1440; dayDelta++; }
    f[0] = String(total % 60);
    f[1] = String(Math.floor(total / 60));
    if (dayDelta !== 0) {
      if (f[2] !== '*' || f[3] !== '*') return null; // monthly/dom + wrap → too risky
      if (f[4] !== '*') { // "*" is daily — a wrap leaves it daily
        var shifted = cronShiftDow(f[4], dayDelta);
        if (shifted === null) return null;
        f[4] = shifted;
      }
    }
    return f.join(' ');
  }
  // ───────────────────────────────────────────────────────────────────────────

  Alpine.data('cronField', function () {
    return {
      presets: [],
      cron: '',
      customKey: 'custom',
      previewUrl: '/-/cron/preview',
      // localizePresets: chips are viewer-local intents; convert to server-local
      // on apply and when computing the active chip. serverUtcOffsetMin is the
      // server's UTC offset in minutes east of UTC (0 = no shift).
      localizePresets: false,
      serverUtcOffsetMin: 0,
      preview: { invalid: false, description: '', loading: false },
      _debounce: null,

      init: function () {
        var cfg = {};
        try { cfg = JSON.parse(this.$el.dataset.config || '{}'); } catch (e) { cfg = {}; }
        // Only render chips that carry a cron — the always-visible input is the
        // custom path, so a legacy empty "Custom…" preset is dropped.
        this.presets = (cfg.presets || []).filter(function (p) { return p && p.cron; });
        this.previewUrl = cfg.previewUrl || '/-/cron/preview';
        this.customKey = cfg.customKey || 'custom';
        this.localizePresets = !!cfg.localizePresets;
        this.serverUtcOffsetMin = parseInt(cfg.serverUtcOffsetMin, 10) || 0;
        this.cron = (cfg.value || '').trim();
        // Refresh the preview whenever the cron changes — covers typing, chip
        // clicks, AND an external value pushed in via x-model (reconfigure).
        this.$watch('cron', function () { this.refresh(); }.bind(this));
        if (this.cron) this.fetchPreview();
      },

      // presetCron returns the cron a chip resolves to: literal, or shifted to
      // server-local time when the chip is a viewer-local intent.
      presetCron: function (preset) {
        var expr = (preset && preset.cron) || '';
        if (!this.localizePresets) return expr;
        return this.localCronToServer(expr);
      },

      // localCronToServer converts a viewer-local cron into the server-local cron
      // the scheduler stores + fires. Falls back to the input unchanged when
      // there's no timezone delta (server tz == viewer tz) or the shift isn't
      // representable.
      localCronToServer: function (localExpr) {
        var viewerOff;
        try {
          viewerOff = -new Date().getTimezoneOffset(); // minutes east of UTC
        } catch (e) {
          return localExpr;
        }
        var deltaMin = this.serverUtcOffsetMin - viewerOff; // viewer-local → server-local
        if (!deltaMin) return localExpr;
        return cronShiftTimeFields(localExpr, deltaMin) || localExpr;
      },

      // presetActive lights a chip when the current cron equals what the chip
      // resolves to — so it stays lit whether clicked, typed, or hydrated, and
      // across timezones.
      presetActive: function (preset) {
        return this.presetCron(preset).trim() === String(this.cron || '').trim();
      },

      applyPreset: function (preset) {
        this.cron = this.presetCron(preset);
      },

      // activeKey is the key of the matching chip (for the optional PresetName
      // hidden field), or the custom key when none match.
      activeKey: function () {
        for (var i = 0; i < this.presets.length; i++) {
          if (this.presetActive(this.presets[i])) return this.presets[i].key;
        }
        return this.customKey;
      },

      // previewText renders the live description, an error hint, or an empty
      // prompt depending on validity + whether anything's been entered.
      previewText: function () {
        if (!String(this.cron || '').trim()) return 'Enter a schedule';
        if (this.preview.invalid) return this.preview.description || 'Not a valid cron expression';
        return this.preview.description || '…';
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
          self.preview = { invalid: false, description: '', loading: false };
          return;
        }
        self.preview.loading = true;
        // Always pass the viewer's IANA timezone — endpoints that localize honor
        // it; the instance-tz endpoint ignores it.
        var tz = '';
        try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch (e) { tz = ''; }
        fetch(this.previewUrl + '?cron=' + encodeURIComponent(expr) + (tz ? '&tz=' + encodeURIComponent(tz) : ''), {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        })
          .then(function (res) { return res.json(); })
          .then(function (data) {
            self.preview = {
              invalid: !(data && data.valid),
              description: (data && data.description) || '',
              loading: false,
            };
          })
          .catch(function () {
            self.preview = { invalid: false, description: 'Preview unavailable', loading: false };
          });
      },
    };
  });
});
