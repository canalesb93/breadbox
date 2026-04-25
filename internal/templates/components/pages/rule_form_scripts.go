package pages

import "fmt"

// ruleFormBootstrap renders the inline Alpine factory for the rule form.
// Extracted from the templ template so the JS body stays plain text and
// doesn't compete with templ's `{ }` interpolation. Mirrors the original
// html/template version byte-for-byte except for the two interpolation
// sites: `existingRule` (toJSON .Rule | "null") and `isEdit` (true|false).
func ruleFormBootstrap(p RuleFormProps) string {
	isEdit := "false"
	if p.IsEdit {
		isEdit = "true"
	}
	return fmt.Sprintf(`<script>
function ruleForm() {
  const fieldTypes = {
    name: 'string', merchant_name: 'string', amount: 'numeric',
    category_primary: 'string', category_detailed: 'string',
    category: 'string',
    pending: 'bool', provider: 'string',
    account_id: 'string', account_name: 'string',
    user_id: 'string', user_name: 'string',
    tags: 'tags',
  };
  const defaultOps = { string: 'contains', numeric: 'gte', bool: 'eq', tags: 'contains' };
  // Operator option sets keyed by fieldType. Kept as labels (not HTML glyphs)
  // because the visual builder renders via x-text now.
  const operatorOptionsByType = {
    string: [
      { value: 'contains',     label: 'contains' },
      { value: 'eq',           label: 'equals' },
      { value: 'neq',          label: 'not equals' },
      { value: 'not_contains', label: 'not contains' },
      { value: 'matches',      label: 'regex' },
      { value: 'in',           label: 'in list' },
    ],
    numeric: [
      { value: 'gte', label: '≥' },
      { value: 'lte', label: '≤' },
      { value: 'eq',  label: '=' },
      { value: 'gt',  label: '>' },
      { value: 'lt',  label: '<' },
      { value: 'neq', label: '≠' },
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
  };
  // The initial condition row renders empty so the user must pick a field
  // explicitly — the operator + value inputs then snap to sensible defaults
  // via onFieldChange().
  const emptyCondition = () => ({ field: '', op: '', value: '' });
  // New action rows start as unpicked drafts so "Action..." is the default
  // and the value input stays disabled until a type is chosen.
  const emptyAction = () => ({ field: '', value: '', error: '' });

  // Action type registry — first-class typed actions match the API's
  // supported action.type values (set_category | add_tag | remove_tag |
  // add_comment). The internal "field" name is a UI alias; we map back
  // at submit time.
  const actionTypes = [
    { value: 'category',   label: 'Set category' },
    { value: 'tag',        label: 'Add tag' },
    { value: 'tag_remove', label: 'Remove tag' },
    { value: 'comment',    label: 'Add comment' },
  ];

  // Tag slug regex must match the server-side validator in
  // internal/service/rules.go (tagSlugPattern).
  const tagSlugRegex = /^[a-z0-9][a-z0-9\-:]*[a-z0-9]$/;

  const existingRule = %s;

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
    // Tolerate unknown types so the form still loads — validation happens at submit.
    return { field: a.type || a.field || '', value: a.category_slug || a.tag_slug || a.content || a.value || '', error: '' };
  }

  // Initialize form from existing rule or defaults
  function initForm() {
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
    let conditions = [], logic = 'and';
    const c = existingRule.conditions;
    if (c && c.and) {
      conditions = c.and.map(sub => ({ field: sub.field || '', op: sub.op || 'contains', value: String(sub.value ?? '') }));
      logic = 'and';
    } else if (c && c.or) {
      conditions = c.or.map(sub => ({ field: sub.field || '', op: sub.op || 'contains', value: String(sub.value ?? '') }));
      logic = 'or';
    } else if (c && c.field) {
      conditions = [{ field: c.field, op: c.op || 'contains', value: String(c.value ?? '') }];
    }
    // else: NULL or empty {} → conditions stays [] (match-all)

    // Parse actions (typed shape: {type, category_slug|tag_slug|content})
    let actions = (existingRule.actions || []).map(actionToForm);
    if (actions.length === 0 && existingRule.category_slug) {
      actions = [{ field: 'category', value: existingRule.category_slug, error: '' }];
    }
    if (actions.length === 0) {
      actions = [emptyAction()];
    }

    // Normalize legacy on_update → on_change so the <select> binds cleanly.
    const trigger = existingRule.trigger === 'on_update' ? 'on_change' : (existingRule.trigger || 'on_create');

    return {
      name: existingRule.name,
      priority: existingRule.priority,
      trigger,
      logic,
      conditions,
      conditions_json: existingRule.conditions ? JSON.stringify(existingRule.conditions, null, 2) : '',
      actions
    };
  }

  // Pipeline-stage presets. Numeric values are conventional, not enforced —
  // users can still pick any integer 0..1000 in the fallback number input.
  // Under priority-ASC ordering, lower stages run first.
  const priorityPresets = [
    { value: 0,   label: 'Baseline',   hint: 'Runs first — broad defaults, seeded rules' },
    { value: 10,  label: 'Standard',   hint: 'Default rule stage' },
    { value: 50,  label: 'Refinement', hint: 'Runs after standard rules — reacts to their output' },
    { value: 100, label: 'Override',   hint: 'Runs last — wins set_category conflicts' },
  ];
  function priorityLabelFor(p) {
    const match = priorityPresets.find(x => x.value === p);
    if (match) return match.hint;
    if (p < 10) return 'Runs very early — before standard rules';
    if (p < 50) return 'Runs with standard rules';
    if (p < 100) return 'Runs as a refinement';
    return 'Runs very late — has the final word';
  }

  return {
    isEdit: %s,
    editingId: existingRule?.id || '',
    nameFocused: false,
    showJsonEditor: false,
    submitting: false,
    formError: '',
    actionTypes,
    priorityPresets,
    form: initForm(),

    fieldType(field) { return fieldTypes[field] || 'string'; },
    operatorOptions(field) {
      // Empty field → show an empty options set so the operator select is
      // visually blank until a field is picked.
      if (!field) return [];
      return operatorOptionsByType[this.fieldType(field)] || operatorOptionsByType.string;
    },
    priorityLabel(p) { return priorityLabelFor(p); },

    restorePageState() {
      const main = document.querySelector('main');
      if (main) { main.style.opacity = ''; main.style.filter = ''; main.style.pointerEvents = ''; }
      if (window.bbProgress) window.bbProgress.finish();
    },

    insertArrow() {
      const el = this.$refs.nameInput;
      const pos = el.selectionStart;
      const before = this.form.name.slice(0, pos);
      const after = this.form.name.slice(el.selectionEnd);
      this.form.name = before + ' → ' + after;
      this.$nextTick(() => { const np = pos + 3; el.focus(); el.setSelectionRange(np, np); });
    },

    // Condition helpers
    onFieldChange(idx) {
      const cond = this.form.conditions[idx];
      // Assigned-category bias: default to eq so the value input renders as
      // the category dropdown rather than a substring match. Power users can
      // still flip to contains/matches/in for slug-based matching.
      if (cond.field === 'category') cond.op = 'eq';
      else cond.op = defaultOps[this.fieldType(cond.field)] || 'contains';
      if (this.fieldType(cond.field) === 'bool') cond.value = 'true';
      else cond.value = '';
      this.syncToJson();
    },
    addCondition() {
      this.form.conditions.push(emptyCondition());
      this.syncToJson();
      this.$nextTick(() => lucide.createIcons());
    },
    removeCondition(idx) {
      this.form.conditions.splice(idx, 1);
      this.syncToJson();
    },

    // Action helpers. Only set_category is singleton (backend enforces one
    // per rule); add_tag / remove_tag / add_comment may repeat.
    isActionUsed(field) {
      if (field !== 'category') return false;
      return this.form.actions.some(a => a.field === field);
    },
    // Block "Add action" only while there's an unpicked draft row.
    canAddAction() {
      if (this.form.actions.some(a => !a.field)) return false;
      return true;
    },
    addActionTooltip() {
      if (this.form.actions.some(a => !a.field)) return 'Pick an action type first';
      return 'Add another action';
    },
    addAction() {
      if (!this.canAddAction()) return;
      this.form.actions.push(emptyAction());
      this.$nextTick(() => lucide.createIcons());
    },
    removeAction(idx) {
      this.form.actions.splice(idx, 1);
    },
    onActionTypeChange(idx) {
      this.form.actions[idx].value = '';
      this.form.actions[idx].error = '';
    },
    // Validate tag slug inline so the error renders before submit
    validateTagSlug(idx) {
      const a = this.form.actions[idx];
      if (!a) return;
      if (a.value && !tagSlugRegex.test(a.value)) {
        a.error = 'Lowercase letters, numbers, hyphens, or colons (e.g. needs-review)';
      } else {
        a.error = '';
      }
    },
    // Flag action combinations that cancel out or are otherwise suspicious
    // so the user sees the problem before hitting submit.
    combinationWarnings() {
      const warnings = [];
      const addTags = this.form.actions.filter(a => a.field === 'tag' && a.value);
      const removeTags = this.form.actions.filter(a => a.field === 'tag_remove' && a.value);
      for (const a of addTags) {
        for (const r of removeTags) {
          if (a.value === r.value) {
            warnings.push(`+"`"+`Tag "${a.value}" is being added and removed — these cancel out.`+"`"+`);
          }
        }
      }
      return warnings;
    },

    // JSON sync
    syncToJson() {
      const json = this.buildConditionsJSON();
      this.form.conditions_json = JSON.stringify(json, null, 2);
    },
    syncFromJson() {
      try {
        const parsed = JSON.parse(this.form.conditions_json);
        if (parsed.and) {
          this.form.logic = 'and';
          this.form.conditions = parsed.and.map(sub => ({ field: sub.field || '', op: sub.op || 'contains', value: String(sub.value ?? '') }));
        } else if (parsed.or) {
          this.form.logic = 'or';
          this.form.conditions = parsed.or.map(sub => ({ field: sub.field || '', op: sub.op || 'contains', value: String(sub.value ?? '') }));
        } else if (parsed.field) {
          this.form.conditions = [{ field: parsed.field, op: parsed.op || 'contains', value: String(parsed.value ?? '') }];
        }
      } catch (e) { /* let them type freely */ }
    },

    // Build the JSON shape the API expects from the visual condition rows.
    // Returns null when the user has no conditions (match-all), so the JSON
    // body sends `+"`"+`conditions: null`+"`"+` and the server stores NULL.
    buildConditionsJSON() {
      const conds = this.form.conditions.filter(c => c.field && c.value !== '');
      if (conds.length === 0) return null;
      const mapped = conds.map(c => {
        let val = c.value;
        const type = this.fieldType(c.field);
        if (type === 'numeric') val = parseFloat(val) || 0;
        else if (type === 'bool') val = val === 'true';
        else if (type === 'tags' && c.op === 'in') {
          // Tags `+"`"+`in`+"`"+` takes an array; UI collects a comma-separated string.
          val = String(val).split(',').map(s => s.trim()).filter(Boolean);
        }
        // string `+"`"+`in`+"`"+` op isn't exposed via the visual builder — advanced
        // users use the JSON editor for that shape.
        return { field: c.field, op: c.op, value: val };
      });
      if (mapped.length === 1) return mapped[0];
      return this.form.logic === 'or' ? { or: mapped } : { and: mapped };
    },

    // Submit
    async submitRule() {
      this.formError = '';
      this.submitting = true;

      let conditions;
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
      let actionError = '';
      this.form.actions.forEach(a => {
        if ((a.field === 'tag' || a.field === 'tag_remove') && a.value && !tagSlugRegex.test(a.value)) {
          a.error = 'Lowercase letters, numbers, hyphens, or colons (e.g. needs-review)';
          actionError = actionError || 'Fix the tag slug before saving.';
        }
      });
      if (actionError) {
        this.formError = actionError;
        this.submitting = false;
        this.restorePageState();
        return;
      }

      // Build typed actions array for the API. Drop incomplete drafts.
      const actions = this.form.actions
        .filter(a => a.field && (a.value !== undefined && a.value !== ''))
        .map(a => {
          if (a.field === 'category') return { type: 'set_category', category_slug: a.value };
          if (a.field === 'tag') return { type: 'add_tag', tag_slug: a.value };
          if (a.field === 'tag_remove') return { type: 'remove_tag', tag_slug: a.value };
          if (a.field === 'comment') return { type: 'add_comment', content: a.value };
          return { type: a.field, category_slug: a.value };
        });
      if (actions.length === 0) {
        this.formError = 'At least one action with a value is required';
        this.submitting = false;
        this.restorePageState();
        return;
      }

      try {
        const body = {
          name: this.form.name,
          conditions,
          actions,
          trigger: this.form.trigger || 'on_create',
          priority: this.form.priority,
        };

        const url = this.isEdit ? '/-/rules/' + this.editingId : '/-/rules';
        const method = this.isEdit ? 'PUT' : 'POST';

        const resp = await fetch(url, {
          method,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body)
        });

        if (!resp.ok) {
          const data = await resp.json();
          this.formError = data.error?.message || data.error || 'Failed to save rule';
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
}
</script>`, ruleJSON(p.Rule), isEdit)
}
