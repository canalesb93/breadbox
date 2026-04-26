// Rule detail Alpine component for /rules/{id}.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Initial categories payload is rendered server-side as
// <script id="rule-detail-data" type="application/json">[...]</script>
// via @templ.JSONScript and parsed once in init().
//
// Back-compat note: the `tx_row` partials rendered inside Recent Applications
// and Matching Transactions reach into `window.__bbCategories` to feed the
// inline categoryPicker (see internal/templates/components/tx_row.templ).
// `showToast` and `quickSetCategory` are likewise expected on the global
// scope by the tx-row inline `x-init` watcher. We expose all three from the
// factory's `init()` (and at module top-level) so the cross-component
// contract continues to hold without forcing tx_row to know about Alpine
// scoping.

// --- Module-level globals consumed by tx_row + base.html ---

// Inline category picker on tx-row fires this on change. Mirrors the
// /transactions and /account/{id} paths; the post-success
// `window.updateRowCategory(...)` call is what makes the avatar icon and
// xl-hidden compact label update without a reload (same shared helper
// loaded from static/js/admin/components/tx_row_helpers.js).
function quickSetCategory(txId, categoryId) {
  if (!categoryId) {
    fetch('/-/transactions/' + txId + '/category', { method: 'DELETE' })
      .then(function (r) {
        if (r.ok || r.status === 204) {
          showToast('Category reset', 'success');
          if (window.updateRowCategory) window.updateRowCategory(txId, '');
        }
      });
    return;
  }
  fetch('/-/transactions/' + txId + '/category', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ category_id: categoryId }),
  })
    .then(function (r) {
      if (r.ok) {
        showToast('Category updated', 'success');
        if (window.updateRowCategory) {
          window.updateRowCategory(txId, (window._slugForCategoryId || function () { return ''; })(categoryId));
        }
      } else {
        r.json().then(function (d) { showToast(d.error?.message || 'Failed', 'error'); });
      }
    });
}

function showToast(message, type) {
  window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
}

// Expose on window so tx-row's inline x-init can find them. (Function
// declarations are already global in the page's main script, but being
// explicit makes the contract obvious to future readers.)
window.quickSetCategory = quickSetCategory;
window.showToast = showToast;

document.addEventListener('alpine:init', function () {
  // Minimal no-op bulk store — tx-row references $store.bulk.processing and
  // .toggle(), but we never enter selection mode on the rule detail page.
  if (!Alpine.store('bulk')) {
    Alpine.store('bulk', {
      selecting: false,
      processing: false,
      sel: [],
      isIn: function () { return false; },
      toggle: function () {},
      toggleMode: function () {},
      clear: function () {},
    });
  }

  Alpine.data('ruleDetail', function () {
    return {
      applying: false,
      applyError: '',
      showApplyModal: false,
      confirmApply: false,
      applyUrl: '',
      toggleUrl: '',

      init: function () {
        // Categories tree → window.__bbCategories for the inline categoryPicker
        // factory in tx_row.templ. Parsed once and exposed globally for the
        // shared component contract.
        var dataEl = document.getElementById('rule-detail-data');
        if (dataEl) {
          try {
            window.__bbCategories = JSON.parse(dataEl.textContent);
          } catch (e) {
            console.error('ruleDetail: failed to parse #rule-detail-data', e);
            window.__bbCategories = null;
          }
        }
        // Apply / toggle URLs come in as data-* attributes on the x-data root.
        this.applyUrl = this.$el.dataset.applyUrl || '';
        this.toggleUrl = this.$el.dataset.toggleUrl || '';
      },

      // Register page-scoped shortcuts so they surface in the `?` help modal.
      // `Cmd+Enter` (toggle) is wired via inline @keydown.meta.enter.window
      // because the global dispatcher short-circuits on modifier keys; this
      // registry entry is visible-only so the help modal advertises it.
      // `Esc` closes the apply modal via @keydown.escape.window on the modal
      // itself — registered here (visible-only) for discoverability.
      registerShortcuts: function () {
        if (!window.Alpine || !Alpine.store('shortcuts')) return;
        var reg = Alpine.store('shortcuts');
        reg.register({
          id: 'rule-detail.save',
          keys: 'cmd+enter',
          description: 'Toggle rule (enable / disable)',
          group: 'Actions',
          scope: 'rule-detail',
          visible: true,
          // no action — dispatched by inline @keydown.meta.enter on the page root
        });
        reg.register({
          id: 'rule-detail.apply',
          keys: 'a',
          description: 'Apply rule to matching transactions',
          group: 'Actions',
          scope: 'rule-detail',
          when: function () {
            return !!document.querySelector('[data-apply-btn]');
          },
          action: function () {
            var btn = document.querySelector('[data-apply-btn]');
            if (btn) btn.click();
          },
        });
        reg.register({
          id: 'rule-detail.esc',
          keys: 'Esc',
          description: 'Close apply dialog',
          group: 'Actions',
          scope: 'rule-detail',
          visible: true,
          // no action — handled by @keydown.escape.window on the modal itself
        });
      },

      unregisterShortcuts: function () {
        if (!window.Alpine || !Alpine.store('shortcuts')) return;
        var reg = Alpine.store('shortcuts');
        reg.unregister('rule-detail.save');
        reg.unregister('rule-detail.apply');
        reg.unregister('rule-detail.esc');
      },

      openApplyModal: function () {
        this.confirmApply = false;
        this.applyError = '';
        this.showApplyModal = true;
      },

      closeApplyModal: function () {
        if (this.applying) return;
        this.showApplyModal = false;
      },

      applyRetroactively: async function () {
        if (!this.confirmApply) return;
        this.applying = true;
        this.applyError = '';
        try {
          const resp = await fetch(this.applyUrl, { method: 'POST' });
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
          setTimeout(function () { location.reload(); }, 800);
        } catch (e) {
          this.applyError = 'Network error: ' + e.message;
          this.applying = false;
        }
      },

      toggleRule: async function () {
        try {
          const resp = await fetch(this.toggleUrl, { method: 'POST' });
          if (resp.ok) location.reload();
        } catch (e) {
          /* ignore */
        }
      },
    };
  });
});
