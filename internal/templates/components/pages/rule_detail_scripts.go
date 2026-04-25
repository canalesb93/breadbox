package pages

import "fmt"

// ruleDetailBootstrap renders the inline <script> blocks that power the
// rule detail page. Extracted from the templ template so the JS bodies stay
// plain text and don't compete with templ's `{ }` interpolation. Mirrors
// the original html/template version byte-for-byte except for the three
// interpolation sites:
//   - window.__bbCategories ← {{toJSON .Categories}}
//   - the apply URL          ← /-/rules/{{.Rule.ID}}/apply
//   - the toggle URL         ← /-/rules/{{.Rule.ID}}/toggle
func ruleDetailBootstrap(p RuleDetailProps) string {
	ruleID := ""
	if p.Rule != nil {
		ruleID = p.Rule.ID
	}
	return fmt.Sprintf(`<script>
// tx-row partials on this page (Recent Applications, Matching transactions)
// share the inline category picker + quickSetCategory from the transactions
// list. Expose the same globals they expect so the picker works here without
// pulling in the full transactions-page bulk machinery.
window.__bbCategories = %s;

document.addEventListener('alpine:init', function() {
  if (!Alpine.store('bulk')) {
    // Minimal no-op bulk store — tx-row references $store.bulk.processing and
    // .toggle(), but we never enter selection mode on the rule detail page.
    Alpine.store('bulk', {
      selecting: false,
      processing: false,
      sel: [],
      isIn: function() { return false; },
      toggle: function() {},
      toggleMode: function() {},
      clear: function() {}
    });
  }
});

function showToast(message, type) {
  window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
}

// Inline category picker on tx-row fires this on change. Matches the impl on
// /transactions and /accounts/{id} — keep in sync if that endpoint evolves.
function quickSetCategory(txId, categoryId) {
  if (!categoryId) {
    fetch('/-/transactions/' + txId + '/category', { method: 'DELETE' })
    .then(function(r) {
      if (r.ok || r.status === 204) showToast('Category reset', 'success');
    });
    return;
  }
  fetch('/-/transactions/' + txId + '/category', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({category_id: categoryId})
  })
  .then(function(r) {
    if (r.ok) showToast('Category updated', 'success');
    else r.json().then(function(d) { showToast(d.error?.message || 'Failed', 'error'); });
  });
}
</script>

<script>
function ruleDetail() {
  return {
    applying: false,
    applyError: '',
    showApplyModal: false,
    confirmApply: false,
    // Register page-scoped shortcuts so they surface in the `+"`"+`?`+"`"+` help modal.
    // `+"`"+`Cmd+Enter`+"`"+` (toggle) is wired via inline @keydown.meta.enter.window
    // because the global dispatcher short-circuits on modifier keys; this
    // registry entry is visible-only so the help modal advertises it.
    // `+"`"+`Esc`+"`"+` closes the apply modal via @keydown.escape.window on the modal
    // itself — registered here (visible-only) for discoverability.
    registerShortcuts() {
      if (!window.Alpine || !Alpine.store('shortcuts')) return;
      var reg = Alpine.store('shortcuts');
      var self = this;
      reg.register({
        id: 'rule-detail.save',
        keys: 'cmd+enter',
        description: 'Toggle rule (enable / disable)',
        group: 'Actions',
        scope: 'rule-detail',
        visible: true
        // no action — dispatched by inline @keydown.meta.enter on the page root
      });
      reg.register({
        id: 'rule-detail.apply',
        keys: 'a',
        description: 'Apply rule to matching transactions',
        group: 'Actions',
        scope: 'rule-detail',
        when: function() {
          return !!document.querySelector('[data-apply-btn]');
        },
        action: function() {
          var btn = document.querySelector('[data-apply-btn]');
          if (btn) btn.click();
        }
      });
      reg.register({
        id: 'rule-detail.esc',
        keys: 'Esc',
        description: 'Close apply dialog',
        group: 'Actions',
        scope: 'rule-detail',
        visible: true
        // no action — handled by @keydown.escape.window on the modal itself
      });
    },
    unregisterShortcuts() {
      if (!window.Alpine || !Alpine.store('shortcuts')) return;
      var reg = Alpine.store('shortcuts');
      reg.unregister('rule-detail.save');
      reg.unregister('rule-detail.apply');
      reg.unregister('rule-detail.esc');
    },
    openApplyModal() {
      this.confirmApply = false;
      this.applyError = '';
      this.showApplyModal = true;
    },
    closeApplyModal() {
      if (this.applying) return;
      this.showApplyModal = false;
    },
    async applyRetroactively() {
      if (!this.confirmApply) return;
      this.applying = true;
      this.applyError = '';
      try {
        const resp = await fetch('/-/rules/%s/apply', { method: 'POST' });
        const data = await resp.json();
        if (!resp.ok) {
          this.applyError = data.error?.message || data.error || 'Failed to apply rule';
          this.applying = false;
          return;
        }
        // Promote success to the global toast (see base.html) so it persists
        // through the reload window and isn't clipped by the closing modal.
        const msg = 'Applied to ' + (data.affected_count || 0) + ' transactions';
        showToast(msg, 'success');
        this.applying = false;
        this.showApplyModal = false;
        // Reload so the updated application list and hit counts show up.
        setTimeout(() => location.reload(), 800);
      } catch (e) {
        this.applyError = 'Network error: ' + e.message;
        this.applying = false;
      }
    },
    async toggleRule() {
      try {
        const resp = await fetch('/-/rules/%s/toggle', { method: 'POST' });
        if (resp.ok) location.reload();
      } catch (e) { /* ignore */ }
    }
  };
}
</script>`, ruleDetailCategoriesJSON(p.Categories), ruleID, ruleID)
}
