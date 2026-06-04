// Agent form Alpine component for /agents/new and /agents/{slug}/edit.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Initial form state is rendered server-side as
//   <script id="agent-form-data" type="application/json">{...}</script>
// via @templ.JSONScript and parsed once in init().
//
// Responsibilities:
//   - Cron picker: clickable presets, manual override, human-readable summary,
//     and the next 3 fire times rendered in the browser's local timezone.
//   - "Advanced" toggle: keeps low-traffic fields (system prompt, max turns,
//     max budget, quiet hours, allowed-tools allowlist) collapsed by default.
//     Opens automatically when editing an agent that already has a non-default
//     value in any of the collapsed fields, so the existing value is visible.

document.addEventListener('alpine:init', function () {
  Alpine.data('agentForm', function () {
    // ---- cron presets -----------------------------------------------------
    //
    // The presets are not just labels — they're the canonical mapping between
    // a short slug and the cron expression we want to write into the input
    // when the user clicks the chip. "off" is special: empty cron value, no
    // scheduled fires. "custom" is also special: it means "I'm going to type
    // a raw expression"; we just unlock the text input.
    var PRESETS = [
      { id: 'off',     label: 'Off',          expr: '',           hint: 'No scheduled runs' },
      { id: 'hourly',  label: 'Every hour',   expr: '0 * * * *',  hint: 'At minute 0 of every hour' },
      { id: 'every6h', label: 'Every 6 h',    expr: '0 */6 * * *', hint: 'Every 6 hours' },
      { id: 'daily',   label: 'Daily 8 AM',   expr: '0 8 * * *',  hint: 'Every day at 08:00' },
      { id: 'weekly',  label: 'Weekly Mon',   expr: '0 8 * * 1',  hint: 'Every Monday at 08:00' },
      { id: 'monthly', label: 'Monthly 1st',  expr: '0 8 1 * *',  hint: 'On the 1st at 08:00' },
      { id: 'custom',  label: 'Custom',       expr: null,         hint: 'Write a raw cron expression' },
    ];

    function presetFromExpr(expr) {
      var trimmed = (expr || '').trim();
      if (!trimmed) return 'off';
      for (var i = 0; i < PRESETS.length; i++) {
        if (PRESETS[i].expr === trimmed) return PRESETS[i].id;
      }
      return 'custom';
    }

    // ---- cron parser ------------------------------------------------------
    //
    // Standard 5-field cron parser. Supports the operators robfig/cron's
    // ParseStandard accepts: numeric (`8`), wildcard (`*`), step (`*/15`,
    // `5/10`), range (`1-5`), and lists (`1,3,5`). Day-of-week 0 and 7 both
    // mean Sunday. Names ("MON", "JAN") are not supported — the presets above
    // cover the common cases and power users can switch to Custom.

    var FIELDS = [
      { name: 'minute', min: 0, max: 59 },
      { name: 'hour',   min: 0, max: 23 },
      { name: 'dom',    min: 1, max: 31 },
      { name: 'month',  min: 1, max: 12 },
      { name: 'dow',    min: 0, max: 6  },
    ];

    function parseField(token, min, max) {
      if (token === '*') {
        return { all: true };
      }
      var parts = token.split(',');
      var values = {};
      for (var i = 0; i < parts.length; i++) {
        var p = parts[i];
        var step = 1;
        var stepIdx = p.indexOf('/');
        if (stepIdx >= 0) {
          step = parseInt(p.slice(stepIdx + 1), 10);
          if (!isFinite(step) || step <= 0) return null;
          p = p.slice(0, stepIdx);
        }
        var lo = min;
        var hi = max;
        if (p === '*' || p === '') {
          // step on wildcard — use the full range
        } else if (p.indexOf('-') > 0) {
          var bits = p.split('-');
          lo = parseInt(bits[0], 10);
          hi = parseInt(bits[1], 10);
          if (!isFinite(lo) || !isFinite(hi)) return null;
        } else {
          var v = parseInt(p, 10);
          if (!isFinite(v)) return null;
          lo = v; hi = v;
        }
        if (lo < min || hi > max || lo > hi) return null;
        for (var x = lo; x <= hi; x += step) values[x] = true;
      }
      return { all: false, values: values };
    }

    function fieldMatches(spec, value) {
      if (spec.all) return true;
      return !!spec.values[value];
    }

    // Standalone "7" tokens in a dow expression — robfig accepts both 0
    // and 7 for Sunday but our parser only understands 0–6, so we rewrite
    // 7 → 0 before parsing. The previous implementation used a global
    // /7/g regex that also corrupted multi-digit substrings (e.g. "17"
    // inside a step expression became "10", and a hypothetical "1-7"
    // range became "1-0"). Walk the token in pieces (split on commas,
    // dashes, slashes) so only an exact "7" gets remapped.
    function rewriteDowSundays(tok) {
      return tok.replace(/(^|[,\-/])7(?=[,\-/]|$)/g, '$10');
    }

    function parseCron(expr) {
      if (typeof expr !== 'string') return null;
      var trimmed = expr.trim();
      if (!trimmed) return null;
      var tokens = trimmed.split(/\s+/);
      if (tokens.length !== 5) return null;
      var parsed = {};
      for (var i = 0; i < FIELDS.length; i++) {
        var f = FIELDS[i];
        var raw = tokens[i];
        if (f.name === 'dow') raw = rewriteDowSundays(raw);
        var spec = parseField(raw, f.min, f.max);
        if (!spec) return null;
        parsed[f.name] = spec;
      }
      return parsed;
    }

    // Compute the next N fire times after `start`. Uses a brute-force minute
    // scan (capped at one year of minutes) because the inputs are short and
    // there's no library code we want to ship.
    function nextFires(spec, start, count) {
      if (!spec) return [];
      var d = new Date(start.getTime());
      d.setSeconds(0, 0);
      d.setMinutes(d.getMinutes() + 1);
      var out = [];
      var safety = 60 * 24 * 366; // one year of minutes
      while (out.length < count && safety-- > 0) {
        var minuteOK = fieldMatches(spec.minute, d.getMinutes());
        var hourOK   = fieldMatches(spec.hour,   d.getHours());
        var monOK    = fieldMatches(spec.month,  d.getMonth() + 1);
        // DOM and DOW: cron's quirky OR logic only kicks in if BOTH fields are
        // restricted. If either is `*`, only the restricted one matters.
        var domSpec = spec.dom, dowSpec = spec.dow;
        var domVal = d.getDate(), dowVal = d.getDay();
        var dayOK;
        if (domSpec.all && dowSpec.all) {
          dayOK = true;
        } else if (domSpec.all) {
          dayOK = fieldMatches(dowSpec, dowVal);
        } else if (dowSpec.all) {
          dayOK = fieldMatches(domSpec, domVal);
        } else {
          dayOK = fieldMatches(domSpec, domVal) || fieldMatches(dowSpec, dowVal);
        }
        if (minuteOK && hourOK && monOK && dayOK) {
          out.push(new Date(d.getTime()));
        }
        d.setMinutes(d.getMinutes() + 1);
      }
      return out;
    }

    // ---- human-readable summary ------------------------------------------

    var DOW_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
    var MONTH_NAMES = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

    function describeField(spec, names) {
      if (spec.all) return 'every';
      var vals = Object.keys(spec.values).map(Number).sort(function (a, b) { return a - b; });
      if (names) return vals.map(function (v) { return names[v]; }).join(', ');
      return vals.join(', ');
    }

    function pad2(n) { return n < 10 ? '0' + n : String(n); }

    function describe(spec) {
      if (!spec) return '';
      var min = spec.minute, hour = spec.hour;
      var dom = spec.dom, mon = spec.month, dow = spec.dow;
      // Detect simple "single fixed time of day" — most presets land here.
      var minSingle = !min.all && Object.keys(min.values).length === 1;
      var hourSingle = !hour.all && Object.keys(hour.values).length === 1;
      if (minSingle && hourSingle) {
        var m = Number(Object.keys(min.values)[0]);
        var h = Number(Object.keys(hour.values)[0]);
        var clock = pad2(h) + ':' + pad2(m);
        if (dom.all && mon.all && dow.all)  return 'Every day at ' + clock;
        if (dom.all && mon.all && !dow.all) return describeField(dow, DOW_NAMES) + ' at ' + clock;
        if (!dom.all && mon.all && dow.all) {
          var days = describeField(dom, null);
          return 'On day ' + days + ' of every month at ' + clock;
        }
        if (!dom.all && !mon.all && dow.all) {
          return describeField(mon, MONTH_NAMES) + ' ' + describeField(dom, null) + ' at ' + clock;
        }
      }
      // Fallback: piecewise summary.
      var parts = [];
      parts.push('Minute ' + describeField(min, null));
      parts.push('Hour ' + describeField(hour, null));
      if (!dom.all) parts.push('Day-of-month ' + describeField(dom, null));
      if (!mon.all) parts.push('Month ' + describeField(mon, MONTH_NAMES));
      if (!dow.all) parts.push('Day-of-week ' + describeField(dow, DOW_NAMES));
      return parts.join(' · ');
    }

    // ---- factory ---------------------------------------------------------

    return {
      // Form values that drive UI behavior. The remaining form fields are
      // plain inputs the server reads via FormValue, so they don't need
      // Alpine state.
      cronText: '',
      activePreset: 'off',
      advancedOpen: false,
      tzLabel: '',
      // Trigger mode is the "is this scheduled or just a saved prompt?"
      // switch. Manual hides cron / sync-complete / enabled — the agent
      // only runs via the Run-now button. Automatic exposes the full
      // trigger config. Defaults to Manual for fresh agents so a brand-new
      // form doesn't surface a bunch of fields the user didn't ask for.
      triggerMode: 'manual',
      // Mirror the auto-trigger inputs in Alpine state so that switching
      // modes preserves the user's prior choices. When the visible inputs
      // are removed via x-if, these survive and re-populate on switch-back.
      runOnSync: false,
      agentEnabled: false,

      // Custom MCP connectors. Each row: {name, url, header_name, secret,
      // has_secret}. secret is the plaintext the user types this session;
      // has_secret reflects whether one is already stored (so we can show a
      // "leave blank to keep" hint and never echo the stored value back).
      connectors: [],

      init: function () {
        // Pull initial state from the @templ.JSONScript("agent-form-data", ...)
        // tag. Missing tag (create mode with no defaults) → use zero values.
        var data = {};
        var tag = document.getElementById('agent-form-data');
        if (tag) {
          try { data = JSON.parse(tag.textContent || '{}'); } catch (e) { data = {}; }
        }
        this.cronText = (data.cron || '').trim();
        this.activePreset = presetFromExpr(this.cronText);
        this.runOnSync = !!data.trigger_on_sync_complete;
        this.agentEnabled = !!data.enabled;
        // An agent is "Automatic" when it has a real auto-trigger
        // configured — a cron expression or a sync-complete hook. The
        // bare `enabled` flag is orthogonal (you can pause an auto agent
        // without losing its config), so a row with enabled=true but no
        // triggers should still land in Manual — otherwise we'd dump
        // users into an empty Automatic block with no schedule to edit.
        this.triggerMode = (this.cronText || this.runOnSync) ? 'auto' : 'manual';

        // Open the Advanced section automatically when any optional
        // string field inside it already has a non-empty value — surfaces
        // existing config without making users hunt for it. Numeric
        // fields (max_turns, max_budget_usd) are intentionally excluded:
        // the form's own "default" (15) doesn't match the service's
        // canonical default (service.DefaultAgentMaxTurns = 10), so any
        // numeric comparison would force-open Advanced for every agent
        // created via the REST API.
        this.connectors = (Array.isArray(data.connectors) ? data.connectors : []).map(function (c) {
          return {
            name: c.name || '',
            url: c.url || '',
            header_name: c.header_name || '',
            secret: '',
            has_secret: !!c.has_secret,
          };
        });

        this.advancedOpen =
          !!(data.system_prompt && data.system_prompt.length) ||
          !!(data.quiet_hours_start && data.quiet_hours_start.length) ||
          !!(data.quiet_hours_end && data.quiet_hours_end.length) ||
          !!(data.allowed_tools && data.allowed_tools.length) ||
          this.connectors.length > 0;

        // Browser-local timezone label — surfaced next to the preview so the
        // user knows the times below are in their tz, even though the server
        // fires the cron in its own local tz. They typically match for
        // self-hosted Breadbox deploys, but if they don't we want it obvious.
        try {
          var tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
          this.tzLabel = tz || '';
        } catch (e) { this.tzLabel = ''; }

        // Auto-derive slug from the name field while the slug is still
        // empty / untouched. Edit mode loads with a non-empty slug, so
        // the wiring treats it as already-touched and stays out of the
        // way. See static/js/admin/slug.js.
        if (window.bbWireSlugInput) window.bbWireSlugInput('agent-name', 'agent-slug');
      },

      // ---- avatar preview ------------------------------------------------
      //
      // Mirrors the server-side agentFormAvatarSrc helper: the agent's
      // DiceBear robot is seeded by its slug, so the preview tile updates
      // live as the operator types (or as the slug auto-derives from the
      // name — slug.js dispatches an `input` event we listen to). Empty
      // slug falls back to the "new-agent" placeholder seed.
      agentAvatarPreviewSrc: function (slug) {
        var seed = (slug || '').trim() || 'new-agent';
        return '/avatars/' + encodeURIComponent(seed) + '?type=agent&size=80';
      },

      // ---- custom connectors --------------------------------------------
      addConnector: function () {
        this.connectors.push({ name: '', url: '', header_name: 'Authorization', secret: '', has_secret: false });
      },
      removeConnector: function (i) {
        this.connectors.splice(i, 1);
      },

      // ---- cron picker ---------------------------------------------------
      presets: PRESETS,

      applyPreset: function (id) {
        var preset = PRESETS.find(function (p) { return p.id === id; });
        if (!preset) return;
        if (preset.expr !== null) {
          this.cronText = preset.expr;
        } else if (!this.cronText.trim()) {
          // Custom selected with an empty field — seed a sensible value
          // so they have something to edit instead of staring at a blank
          // box. If they already typed something, leave it alone.
          this.cronText = '0 8 * * *';
        }
        // Re-derive activePreset from the final text so the chip set
        // reflects what's actually in the input (e.g. Custom click that
        // seeds 0 8 * * * highlights "Daily 8 AM" instead of "Custom").
        this.activePreset = presetFromExpr(this.cronText);
      },

      onCronInput: function () {
        // Re-derive the active preset from the literal expression. Lets
        // the user type "0 8 * * *" and see the "Daily 8 AM" chip light
        // up. The input is always editable (chips are pure shortcuts),
        // so this never locks the user out mid-typing — earlier versions
        // of this factory flipped the input to readonly when a typed
        // expression matched a preset, which froze the caret.
        this.activePreset = presetFromExpr(this.cronText);
      },

      cronPreview: function () {
        var spec = parseCron(this.cronText);
        if (!spec) {
          if (!this.cronText.trim()) {
            return { ok: true, off: true, description: '', fires: [] };
          }
          return { ok: false, off: false, description: 'Invalid cron expression', fires: [] };
        }
        var fires = nextFires(spec, new Date(), 3);
        var formatter;
        try {
          formatter = new Intl.DateTimeFormat(undefined, {
            weekday: 'short', month: 'short', day: 'numeric',
            hour: 'numeric', minute: '2-digit',
          });
        } catch (e) {
          formatter = { format: function (d) { return d.toString(); } };
        }
        return {
          ok: true,
          off: false,
          description: describe(spec),
          fires: fires.map(function (d) { return formatter.format(d); }),
        };
      },
    };
  });
});
