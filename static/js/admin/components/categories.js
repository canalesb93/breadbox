// Categories page Alpine component for /categories.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// Owns the per-instance filter / expansion state and the j/k focus ring
// for the category tree. The shared base.html shortcuts dispatcher routes
// j/k/Enter/Space/e/n to the registered handlers when the page scope is
// 'categories' (set by the sibling x-init scope-setter div in templ).
//
// Keyboard registrations and the document-level click listener live at
// module scope under one alpine:init — they're global by design and
// shouldn't re-register every time the component mounts.
(function () {
  var FOCUSED_CLASS = 'bb-tx-row--focused'; // reuse the tx-list focus styling

  // Reads the DOM for visible category rows (two levels: primary row +
  // subcategory rows inside each expanded x-collapse). offsetParent !== null
  // respects both the filter (x-show on parent/child wrappers) and the
  // collapsed state (x-collapse toggles display) — collapsed children
  // simply won't appear here.
  function visibleRows() {
    var all = document.querySelectorAll('[data-category-row]');
    var out = [];
    for (var i = 0; i < all.length; i++) {
      if (all[i].offsetParent !== null) out.push(all[i]);
    }
    return out;
  }

  function clearFocus() {
    var rows = document.querySelectorAll('[data-category-row].' + FOCUSED_CLASS);
    for (var i = 0; i < rows.length; i++) rows[i].classList.remove(FOCUSED_CLASS);
  }

  // Module-level pointer to the live page-component instance, updated in
  // init(). Lets the keyboard shortcuts and click listener — both registered
  // once at alpine:init — call methods on the current Alpine instance.
  var instance = null;

  document.addEventListener('alpine:init', function () {
    Alpine.data('categoriesPage', function () {
      return {
        // --- Filter + expansion state (used by inline x-show / x-collapse) ---
        filter: '',
        expanded: {},

        // --- Keyboard navigation state ---
        focusedIdx: -1,

        init: function () {
          instance = this;
        },

        destroy: function () {
          if (instance === this) instance = null;
        },

        // --- Filter + expansion methods (called from inline x-* expressions) ---
        toggle: function (id) {
          this.expanded[id] = !this.expanded[id];
        },

        isOpen: function (id) {
          return !!this.expanded[id] || !!this.filter;
        },

        matches: function (el) {
          if (!this.filter) return true;
          return (el.dataset.search || '').includes(this.filter.toLowerCase());
        },

        // --- Keyboard navigation methods ---
        setFocus: function (idx) {
          var rows = visibleRows();
          if (!rows.length) { this.focusedIdx = -1; return; }
          if (idx < 0) idx = 0;
          if (idx >= rows.length) idx = rows.length - 1;
          clearFocus();
          rows[idx].classList.add(FOCUSED_CLASS);
          this.focusedIdx = idx;
          rows[idx].scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        },

        next: function () {
          var rows = visibleRows();
          if (!rows.length) return;
          this.setFocus(this.focusedIdx < 0 ? 0 : this.focusedIdx + 1);
        },

        prev: function () {
          var rows = visibleRows();
          if (!rows.length) return;
          this.setFocus(this.focusedIdx < 0 ? 0 : this.focusedIdx - 1);
        },

        // Returns the row a keyboard action should target — the j/k-tracked
        // row when one is set, otherwise any row the user has Tab-focused.
        // Keeps Enter/Space/e working for users who reach the tree via Tab
        // instead of j/k.
        currentRow: function () {
          var rows = visibleRows();
          if (this.focusedIdx >= 0 && this.focusedIdx < rows.length) return rows[this.focusedIdx];
          var active = document.activeElement;
          if (active && active.hasAttribute && active.hasAttribute('data-category-row')) return active;
          return null;
        },

        // Activate the focused row: expand/collapse a primary via click
        // (which fires the @click="toggle('<id>')" handler on the row);
        // jump to the filtered transactions list for a subcategory.
        activateRow: function () {
          var row = this.currentRow();
          if (!row) return;
          var level = row.getAttribute('data-category-level');
          if (level === 'primary') {
            row.click();
            return;
          }
          if (level === 'sub') {
            var slug = row.getAttribute('data-category-slug');
            if (slug) window.location.href = '/transactions?category=' + encodeURIComponent(slug);
          }
        },

        editRow: function () {
          var row = this.currentRow();
          if (!row) return;
          var id = row.getAttribute('data-category-id');
          if (id) window.location.href = '/categories/' + encodeURIComponent(id) + '/edit';
        },
      };
    });

    // Page-scope keyboard shortcuts — registered once, dispatched by
    // base.html's global handler when the active scope is 'categories'.
    // The handlers delegate to whichever categoriesPage instance is live.
    var reg = Alpine.store('shortcuts');
    if (!reg) return;

    reg.register({
      id: 'categories.next',
      keys: 'j',
      description: 'Move down',
      group: 'Navigation',
      scope: 'categories',
      action: function () { if (instance) instance.next(); },
    });

    reg.register({
      id: 'categories.prev',
      keys: 'k',
      description: 'Move up',
      group: 'Navigation',
      scope: 'categories',
      action: function () { if (instance) instance.prev(); },
    });

    reg.register({
      id: 'categories.activate',
      keys: 'Enter',
      description: 'Expand primary / open subcategory',
      group: 'Actions',
      scope: 'categories',
      action: function () { if (instance) instance.activateRow(); },
    });

    reg.register({
      id: 'categories.activate.space',
      keys: ' ', // Space — e.key is a literal space
      description: 'Expand / open (Space)',
      group: 'Actions',
      scope: 'categories',
      visible: false, // Enter is the canonical binding shown in help
      action: function () { if (instance) instance.activateRow(); },
    });

    reg.register({
      id: 'categories.edit',
      keys: 'e',
      description: 'Edit focused category',
      group: 'Actions',
      scope: 'categories',
      action: function () { if (instance) instance.editRow(); },
    });

    reg.register({
      id: 'categories.new',
      keys: 'n',
      description: 'New category',
      group: 'Actions',
      scope: 'categories',
      // Shadows the global `n+_` chord while on /categories via the
      // page-scope-wins guard in base.html's hasChordStartingWith
      // (added in M4.2). Clicks the Add Category link so middle-click
      // / cmd-click semantics aren't silently bypassed.
      action: function () {
        var btn = document.querySelector('[data-new-category]');
        if (btn) btn.click();
      },
    });
  });

  // When the user clicks a row (to expand/collapse a primary or select
  // a sub), sync the j/k ring to that row so subsequent keyboard nav
  // picks up from there — matches the tx-list convention.
  document.addEventListener('click', function (e) {
    var row = e.target && e.target.closest && e.target.closest('[data-category-row]');
    if (!row) return;
    // Ignore clicks on action buttons/links inside the row; those
    // stop propagation already, but belt-and-suspenders in case a
    // future affordance forgets.
    if (e.target.closest('a, button')) return;
    var rows = visibleRows();
    var idx = rows.indexOf(row);
    if (idx < 0) return;
    clearFocus();
    row.classList.add(FOCUSED_CLASS);
    if (instance) instance.focusedIdx = idx;
  });
})();
