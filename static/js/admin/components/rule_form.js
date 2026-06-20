// Rule form Alpine component for /rules/new and /rules/{id}/edit.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// The existing rule (only when editing) is rendered server-side as
//   <script id="rule-form-data" type="application/json">{...}</script>
// via @templ.JSONScript and parsed once in init(). When creating a new
// rule, the script tag is omitted and the form starts blank.
//
// `isEdit` is a scalar so it travels via a `data-is-edit` attribute on
// the x-data root and is read off `this.$el.dataset.isEdit` in init().
//
// FlatCategories and Tags continue to be rendered as server-side
// <option>/<datalist> tags in rule_form.templ — the JS factory never
// needs them because the form just collects free-text/slug values.
document.addEventListener('alpine:init', function () {
  Alpine.data('ruleForm', function () {
    var fieldTypes = {
      name: 'string', merchant_name: 'string', amount: 'numeric',
      category_primary: 'string', category_detailed: 'string',
      category: 'string',
      pending: 'bool', provider: 'string',
      account_id: 'string', account_name: 'string',
      user_id: 'string', user_name: 'string',
      tags: 'tags',
      // Date-part fields are numeric, derived server-side from the tz-naive date.
      day_of_month: 'numeric', month: 'numeric',
      day_of_week: 'numeric', day_of_year: 'numeric',
      // "metadata" is the visual-builder field for a metadata.<key> leaf. The
      // dotted field is assembled at submit time from cond.field + cond.key.
      metadata: 'metadata',
    };
    var defaultOps = { string: 'contains', numeric: 'gte', bool: 'eq', tags: 'contains', metadata: 'eq' };

    // metadataFieldPrefix mirrors service.metadataFieldPrefix. A leaf with
    // field "metadata.<key>" reads that key from the transaction's metadata blob.
    var metadataFieldPrefix = 'metadata.';
    // Operators that take no value (presence test) — the value input is hidden.
    var metadataPresenceOps = { exists: true, not_exists: true };
    // Operator option sets keyed by fieldType. Kept as labels (not HTML glyphs)
    // because the visual builder renders via x-text now.
    var operatorOptionsByType = {
      string: [
        { value: 'contains',     label: 'contains' },
        { value: 'eq',           label: 'equals' },
        { value: 'neq',          label: 'not equals' },
        { value: 'not_contains', label: 'not contains' },
        { value: 'matches',      label: 'regex' },
        { value: 'in',           label: 'in list' },
      ],
      numeric: [
        { value: 'gte',     label: '≥' },
        { value: 'lte',     label: '≤' },
        { value: 'eq',      label: '=' },
        { value: 'gt',      label: '>' },
        { value: 'lt',      label: '<' },
        { value: 'neq',     label: '≠' },
        { value: 'approx',  label: '≈ ±' },
        { value: 'between', label: 'between' },
      ],
      bool: [
        { value: 'eq',  label: 'is' },
        { value: 'neq', label: 'is not' },
      ],
      tags: [
        { value: 'contains',     label: 'has' },
        { value: 'not_contains', label: 'does not have' },
        { value: 'in',           label: 'has any of' },
      ],
      metadata: [
        { value: 'eq',           label: 'equals' },
        { value: 'neq',          label: 'not equals' },
        { value: 'contains',     label: 'contains' },
        { value: 'not_contains', label: 'not contains' },
        { value: 'matches',      label: 'regex' },
        { value: 'in',           label: 'in list' },
        { value: 'gt',           label: '>' },
        { value: 'gte',          label: '≥' },
        { value: 'lt',           label: '<' },
        { value: 'lte',          label: '≤' },
        { value: 'exists',       label: 'exists' },
        { value: 'not_exists',   label: 'does not exist' },
      ],
    };
    // The initial condition row renders empty so the user must pick a field
    // explicitly — the operator + value inputs then snap to sensible defaults
    // via onFieldChange().
    // `key` carries the metadata key for a metadata.<key> leaf (unused by every
    // other field type).
    // `tolerance` is the ± window for the numeric `approx` operator; `min`/`max`
    // bound the `between` operator. Unused by every other field/operator.
    function emptyCondition() { return { field: '', op: '', value: '', key: '', tolerance: '', min: '', max: '' }; }
    // New action rows start as unpicked drafts so "Action..." is the default
    // and the value input stays disabled until a type is chosen. `key` /
    // `valueType` are only used by the metadata actions.
    function emptyAction() { return { field: '', value: '', key: '', valueType: 'text', error: '' }; }

    // Action type registry — first-class typed actions match the API's
    // supported action.type values (set_category | add_tag | remove_tag |
    // add_comment | set_metadata | remove_metadata). The internal "field" name
    // is a UI alias; we map back at submit time.
    var actionTypes = [
      { value: 'category',         label: 'Set category' },
      { value: 'tag',             label: 'Add tag' },
      { value: 'tag_remove',      label: 'Remove tag' },
      { value: 'comment',         label: 'Add comment' },
      { value: 'metadata_set',    label: 'Set metadata' },
      { value: 'metadata_remove', label: 'Remove metadata' },
    ];

    // Tag slug regex must match the server-side validator in
    // internal/service/rules.go (tagSlugPattern).
    var tagSlugRegex = /^[a-z0-9][a-z0-9\-:]*[a-z0-9]$/;

    // Map the API's typed action shape onto the form's UI-side {field,value} pair.
    function actionToForm(a) {
      if (a.type === 'set_category' || a.field === 'category') {
        return { field: 'category', value: a.category_slug || a.value || '', error: '' };
      }
      if (a.type === 'add_tag') {
        return { field: 'tag', value: a.tag_slug || '', error: '' };
      }
      if (a.type === 'remove_tag') {
        return { field: 'tag_remove', value: a.tag_slug || '', error: '' };
      }
      if (a.type === 'add_comment') {
        return { field: 'comment', value: a.content || '', error: '' };
      }
      if (a.type === 'set_metadata') {
        // Recover the value's type so the editor renders the right control and
        // re-submits the same JSON type.
        var vt = 'text', val = a.metadata_value;
        if (typeof val === 'boolean') { vt = 'boolean'; val = val ? 'true' : 'false'; }
        else if (typeof val === 'number') { vt = 'number'; val = String(val); }
        else if (val == null) { val = ''; }
        else if (typeof val !== 'string') { val = JSON.stringify(val); }
        return { field: 'metadata_set', key: a.metadata_key || '', value: String(val), valueType: vt, error: '' };
      }
      if (a.type === 'remove_metadata') {
        return { field: 'metadata_remove', key: a.metadata_key || '', value: '', valueType: 'text', error: '' };
      }
      // Tolerate unknown types so the form still loads — validation happens at submit.
      return { field: a.type || a.field || '', value: a.category_slug || a.tag_slug || a.content || a.value || '', error: '' };
    }

    // loadCondition maps a stored condition leaf into the visual-builder shape,
    // splitting a "metadata.<key>" field back into field="metadata" + key. Array
    // values (the `in` operator) render as a comma-joined string.
    function loadCondition(sub) {
      var field = sub.field || '';
      var key = '';
      if (field.indexOf(metadataFieldPrefix) === 0) {
        key = field.slice(metadataFieldPrefix.length);
        field = 'metadata';
      }
      var value = sub.value;
      if (Array.isArray(value)) value = value.join(', ');
      return {
        field: field,
        op: sub.op || 'contains',
        value: String(value == null ? '' : value),
        key: key,
        // Recover the approx ± window and between bounds so an edit round-trips.
        tolerance: sub.tolerance == null ? '' : String(sub.tolerance),
        min: sub.min == null ? '' : String(sub.min),
        max: sub.max == null ? '' : String(sub.max),
      };
    }

    // Initialize form from existing rule or defaults
    function initForm(existingRule) {
      if (!existingRule) {
        return {
          name: '',
          priority: 10,
          trigger: 'on_create',
          logic: 'and',
          conditions: [emptyCondition()],
          conditions_json: '',
          actions: [emptyAction()]
        };
      }

      // Parse conditions. NULL/empty conditions = match-all → empty conditions array
      // and the form renders the "All transactions" banner.
      var conditions = [], logic = 'and';
      var c = existingRule.conditions;
      if (c && c.and) {
        conditions = c.and.map(loadCondition);
        logic = 'and';
      } else if (c && c.or) {
        conditions = c.or.map(loadCondition);
        logic = 'or';
      } else if (c && c.field) {
        conditions = [loadCondition(c)];
      }
      // else: NULL or empty {} → conditions stays [] (match-all)

      // Parse actions (typed shape: {type, category_slug|tag_slug|content})
      var actions = (existingRule.actions || []).map(actionToForm);
      if (actions.length === 0 && existingRule.category_slug) {
        actions = [{ field: 'category', value: existingRule.category_slug, error: '' }];
      }
      if (actions.length === 0) {
        actions = [emptyAction()];
      }

      // Normalize legacy on_update → on_change so the <select> binds cleanly.
      var trigger = existingRule.trigger === 'on_update' ? 'on_change' : (existingRule.trigger || 'on_create');

      return {
        name: existingRule.name,
        priority: existingRule.priority,
        trigger: trigger,
        logic: logic,
        conditions: conditions,
        conditions_json: existingRule.conditions ? JSON.stringify(existingRule.conditions, null, 2) : '',
        actions: actions
      };
    }

    // Pipeline-stage presets. Numeric values are conventional, not enforced —
    // users can still pick any integer 0..1000 in the fallback number input.
    // Under priority-ASC ordering, lower stages run first.
    var priorityPresets = [
      { value: 0,   label: 'Baseline',   hint: 'Runs first — broad defaults, seeded rules' },
      { value: 10,  label: 'Standard',   hint: 'Default rule stage' },
      { value: 50,  label: 'Refinement', hint: 'Runs after standard rules — reacts to their output' },
      { value: 100, label: 'Override',   hint: 'Runs last — wins set_category conflicts' },
    ];
    function priorityLabelFor(p) {
      var match = priorityPresets.find(function (x) { return x.value === p; });
      if (match) return match.hint;
      if (p < 10) return 'Runs very early — before standard rules';
      if (p < 50) return 'Runs with standard rules';
      if (p < 100) return 'Runs as a refinement';
      return 'Runs very late — has the final word';
    }

    return {
      isEdit: false,
      editingId: '',
      nameFocused: false,
      showJsonEditor: false,
      submitting: false,
      formError: '',
      actionTypes: actionTypes,
      priorityPresets: priorityPresets,
      form: initForm(null),

      init: function () {
        // Existing rule (edit mode) arrives via @templ.JSONScript("rule-form-data", p.Rule).
        // The script tag is only emitted when editing — so absence === create mode.
        var existingRule = null;
        var dataEl = document.getElementById('rule-form-data');
        if (dataEl) {
          try {
            existingRule = JSON.parse(dataEl.textContent);
          } catch (e) {
            console.error('ruleForm: failed to parse #rule-form-data', e);
            existingRule = null;
          }
        }
        // Edit-mode flag arrives as a data-* attribute on the x-data root.
        this.isEdit = this.$el.dataset.isEdit === 'true';
        this.editingId = (existingRule && existingRule.id) || '';
        this.form = initForm(existingRule);
      },

      fieldType: function (field) { return fieldTypes[field] || 'string'; },
      operatorOptions: function (field) {
        // Empty field → show an empty options set so the operator select is
        // visually blank until a field is picked.
        if (!field) return [];
        return operatorOptionsByType[this.fieldType(field)] || operatorOptionsByType.string;
      },
      priorityLabel: function (p) { return priorityLabelFor(p); },

      restorePageState: function () {
        var main = document.querySelector('main');
        if (main) { main.style.opacity = ''; main.style.filter = ''; main.style.pointerEvents = ''; }
        if (window.bbProgress) window.bbProgress.finish();
      },

      insertArrow: function () {
        var el = this.$refs.nameInput;
        var pos = el.selectionStart;
        var before = this.form.name.slice(0, pos);
        var after = this.form.name.slice(el.selectionEnd);
        this.form.name = before + ' → ' + after;
        var self = this;
        this.$nextTick(function () { var np = pos + 3; el.focus(); el.setSelectionRange(np, np); });
      },

      // Condition helpers
      onFieldChange: function (idx) {
        var cond = this.form.conditions[idx];
        // Assigned-category bias: default to eq so the value input renders as
        // the category dropdown rather than a substring match. Power users can
        // still flip to contains/matches/in for slug-based matching.
        if (cond.field === 'category') cond.op = 'eq';
        else cond.op = defaultOps[this.fieldType(cond.field)] || 'contains';
        if (this.fieldType(cond.field) === 'bool') cond.value = 'true';
        else cond.value = '';
        // Reset the approx/between params so a stale tolerance/bound doesn't ride
        // along after switching fields.
        cond.tolerance = ''; cond.min = ''; cond.max = '';
        // Reset the metadata key when leaving/entering the metadata field so a
        // stale key doesn't ride along on a non-metadata leaf.
        if (cond.field !== 'metadata') cond.key = '';
        this.syncToJson();
      },
      addCondition: function () {
        this.form.conditions.push(emptyCondition());
        this.syncToJson();
        this.$nextTick(function () { lucide.createIcons(); });
      },
      removeCondition: function (idx) {
        this.form.conditions.splice(idx, 1);
        this.syncToJson();
      },

      // Action helpers. Only set_category is singleton (backend enforces one
      // per rule); add_tag / remove_tag / add_comment may repeat.
      isActionUsed: function (field) {
        if (field !== 'category') return false;
        return this.form.actions.some(function (a) { return a.field === field; });
      },
      // Block "Add action" only while there's an unpicked draft row.
      canAddAction: function () {
        if (this.form.actions.some(function (a) { return !a.field; })) return false;
        return true;
      },
      addActionTooltip: function () {
        if (this.form.actions.some(function (a) { return !a.field; })) return 'Pick an action type first';
        return 'Add another action';
      },
      addAction: function () {
        if (!this.canAddAction()) return;
        this.form.actions.push(emptyAction());
        this.$nextTick(function () { lucide.createIcons(); });
      },
      removeAction: function (idx) {
        this.form.actions.splice(idx, 1);
      },
      onActionTypeChange: function (idx) {
        var a = this.form.actions[idx];
        a.value = '';
        a.error = '';
        a.key = '';
        a.valueType = 'text';
      },
      // When the metadata value-type switches, reset the value so a stale
      // string doesn't carry into a boolean/number control.
      onMetadataValueTypeChange: function (idx) {
        var a = this.form.actions[idx];
        a.value = a.valueType === 'boolean' ? 'true' : '';
        a.error = '';
      },
      // Validate tag slug inline so the error renders before submit
      validateTagSlug: function (idx) {
        var a = this.form.actions[idx];
        if (!a) return;
        if (a.value && !tagSlugRegex.test(a.value)) {
          a.error = 'Lowercase letters, numbers, hyphens, or colons (e.g. needs-review)';
        } else {
          a.error = '';
        }
      },
      // Flag action combinations that cancel out or are otherwise suspicious
      // so the user sees the problem before hitting submit.
      combinationWarnings: function () {
        var warnings = [];
        var addTags = this.form.actions.filter(function (a) { return a.field === 'tag' && a.value; });
        var removeTags = this.form.actions.filter(function (a) { return a.field === 'tag_remove' && a.value; });
        for (var i = 0; i < addTags.length; i++) {
          for (var j = 0; j < removeTags.length; j++) {
            if (addTags[i].value === removeTags[j].value) {
              warnings.push('Tag "' + addTags[i].value + '" is being added and removed — these cancel out.');
            }
          }
        }
        var setMeta = this.form.actions.filter(function (a) { return a.field === 'metadata_set' && a.key; });
        var rmMeta = this.form.actions.filter(function (a) { return a.field === 'metadata_remove' && a.key; });
        for (var m = 0; m < setMeta.length; m++) {
          for (var n = 0; n < rmMeta.length; n++) {
            if (setMeta[m].key.trim() === rmMeta[n].key.trim()) {
              warnings.push('Metadata key "' + setMeta[m].key.trim() + '" is being set and removed — these cancel out.');
            }
          }
        }
        return warnings;
      },

      // JSON sync
      syncToJson: function () {
        var json = this.buildConditionsJSON();
        this.form.conditions_json = JSON.stringify(json, null, 2);
      },
      syncFromJson: function () {
        try {
          var parsed = JSON.parse(this.form.conditions_json);
          if (parsed.and) {
            this.form.logic = 'and';
            this.form.conditions = parsed.and.map(loadCondition);
          } else if (parsed.or) {
            this.form.logic = 'or';
            this.form.conditions = parsed.or.map(loadCondition);
          } else if (parsed.field) {
            this.form.conditions = [loadCondition(parsed)];
          }
        } catch (e) { /* let them type freely */ }
      },

      // Build the JSON shape the API expects from the visual condition rows.
      // Returns null when the user has no conditions (match-all), so the JSON
      // body sends `conditions: null` and the server stores NULL.
      buildConditionsJSON: function () {
        var self = this;
        var conds = this.form.conditions.filter(function (c) {
          if (!c.field) return false;
          if (self.fieldType(c.field) === 'metadata') {
            // A metadata leaf needs a key; presence ops (exists/not_exists)
            // need no value, every other op does.
            if (!c.key || !c.key.trim()) return false;
            if (metadataPresenceOps[c.op]) return true;
            return c.value !== '';
          }
          if (self.fieldType(c.field) === 'numeric') {
            // `between` carries min+max instead of a single value; `approx`
            // needs both a center value and a ± tolerance.
            if (c.op === 'between') return c.min !== '' && c.max !== '';
            if (c.op === 'approx') return c.value !== '' && c.tolerance !== '';
          }
          return c.value !== '';
        });
        if (conds.length === 0) return null;
        var mapped = conds.map(function (c) {
          var val = c.value;
          var type = self.fieldType(c.field);
          if (type === 'metadata') {
            // Assemble the dotted field; presence ops drop the value entirely.
            // `in` collects a comma-separated list into an array. Other ops keep
            // the value as a string — the server's metadata eval coerces by
            // operator (numeric ops parse it; eq/neq stringify-compare).
            var field = metadataFieldPrefix + c.key.trim();
            if (metadataPresenceOps[c.op]) return { field: field, op: c.op };
            var mval = val;
            if (c.op === 'in') mval = String(val).split(',').map(function (s) { return s.trim(); }).filter(Boolean);
            return { field: field, op: c.op, value: mval };
          }
          if (type === 'numeric') {
            if (c.op === 'between') {
              return { field: c.field, op: 'between', min: parseFloat(c.min) || 0, max: parseFloat(c.max) || 0 };
            }
            if (c.op === 'approx') {
              return { field: c.field, op: 'approx', value: parseFloat(val) || 0, tolerance: parseFloat(c.tolerance) || 0 };
            }
            val = parseFloat(val) || 0;
          }
          else if (type === 'bool') val = val === 'true';
          else if (type === 'tags' && c.op === 'in') {
            // Tags `in` takes an array; UI collects a comma-separated string.
            val = String(val).split(',').map(function (s) { return s.trim(); }).filter(Boolean);
          }
          // string `in` op isn't exposed via the visual builder — advanced
          // users use the JSON editor for that shape.
          return { field: c.field, op: c.op, value: val };
        });
        if (mapped.length === 1) return mapped[0];
        return this.form.logic === 'or' ? { or: mapped } : { and: mapped };
      },

      // Submit
      submitRule: async function () {
        this.formError = '';
        this.submitting = true;

        var conditions;
        if (this.showJsonEditor && this.form.conditions_json.trim()) {
          try {
            conditions = JSON.parse(this.form.conditions_json);
            // Treat empty object as match-all (server stores NULL).
            if (conditions && typeof conditions === 'object' && Object.keys(conditions).length === 0) {
              conditions = null;
            }
          } catch (e) {
            this.formError = 'Invalid JSON in conditions: ' + e.message;
            this.submitting = false;
            this.restorePageState();
            return;
          }
        } else {
          conditions = this.buildConditionsJSON(); // null when no conditions = match-all
        }

        // Per-action validation (tag slug shape, missing values).
        var actionError = '';
        this.form.actions.forEach(function (a) {
          if ((a.field === 'tag' || a.field === 'tag_remove') && a.value && !tagSlugRegex.test(a.value)) {
            a.error = 'Lowercase letters, numbers, hyphens, or colons (e.g. needs-review)';
            actionError = actionError || 'Fix the tag slug before saving.';
          }
          // A number-typed metadata value must parse — otherwise it would be
          // silently stored as a string, contradicting the chosen type.
          if (a.field === 'metadata_set' && a.valueType === 'number' && a.value !== '' && isNaN(parseFloat(a.value))) {
            a.error = 'Enter a number, or switch the type to Text.';
            actionError = actionError || 'Fix the numeric metadata value before saving.';
          }
        });
        if (actionError) {
          this.formError = actionError;
          this.submitting = false;
          this.restorePageState();
          return;
        }

        // Build typed actions array for the API. Drop incomplete drafts.
        // Metadata actions are kept on a non-empty key (remove_metadata needs no
        // value; set_metadata may legitimately store an empty string).
        var actions = this.form.actions
          .filter(function (a) {
            if (!a.field) return false;
            if (a.field === 'metadata_set' || a.field === 'metadata_remove') return !!(a.key && a.key.trim());
            return a.value !== undefined && a.value !== '';
          })
          .map(function (a) {
            if (a.field === 'category') return { type: 'set_category', category_slug: a.value };
            if (a.field === 'tag') return { type: 'add_tag', tag_slug: a.value };
            if (a.field === 'tag_remove') return { type: 'remove_tag', tag_slug: a.value };
            if (a.field === 'comment') return { type: 'add_comment', content: a.value };
            if (a.field === 'metadata_set') {
              // Coerce the value to its declared JSON type so the stored blob is
              // typed (boolean true, number 100) rather than always a string.
              var v = a.value;
              if (a.valueType === 'number') { var n = parseFloat(a.value); v = isNaN(n) ? a.value : n; }
              else if (a.valueType === 'boolean') { v = a.value === 'true'; }
              return { type: 'set_metadata', metadata_key: a.key.trim(), metadata_value: v };
            }
            if (a.field === 'metadata_remove') return { type: 'remove_metadata', metadata_key: a.key.trim() };
            return { type: a.field, category_slug: a.value };
          });
        if (actions.length === 0) {
          this.formError = 'At least one action with a value is required';
          this.submitting = false;
          this.restorePageState();
          return;
        }

        try {
          var body = {
            name: this.form.name,
            conditions: conditions,
            actions: actions,
            trigger: this.form.trigger || 'on_create',
            priority: this.form.priority,
          };

          var url = this.isEdit ? '/-/rules/' + this.editingId : '/-/rules';
          var method = this.isEdit ? 'PUT' : 'POST';

          var resp = await fetch(url, {
            method: method,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
          });

          if (!resp.ok) {
            var data = await resp.json();
            this.formError = (data.error && data.error.message) || data.error || 'Failed to save rule';
            this.submitting = false;
            this.restorePageState();
            return;
          }

          window.location.href = '/rules';
        } catch (e) {
          this.formError = 'Network error: ' + e.message;
          this.submitting = false;
          this.restorePageState();
        }
      }
    };
  });
});
