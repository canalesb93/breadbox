// Shared Alpine factory for the inline category picker.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// Drop-in replacement for the legacy `function categoryPicker(config) { ... }`
// factory previously embedded inline in internal/templates/components/category_picker.templ.
// Used across:
//
//   - /categories (parent picker on the create form)        — mode: 'assign'
//   - /transactions (filter bar)                            — mode: 'filter'
//   - /transactions/{id} (assign sidebar)                   — mode: 'assign'
//   - tx-row inline picker (per-row quick set)              — mode: 'assign'
//   - /accounts/{id} (filter bar)                           — mode: 'filter'
//
// The factory takes NO arguments — all configuration flows via `data-*` attrs
// on the root element. Categories come from one of:
//
//   - `data-categories-source` → ID of a <script type="application/json"> tag
//     emitted by `@templ.JSONScript("...", p.Categories)`. Useful when the
//     page already has the tree as a server-side prop (e.g. category_form).
//   - Fallback: `window.__bbCategories` — seeded by the host page's component
//     module (transactions.templ inline bootstrap, account_detail.js,
//     transaction_detail.js, rule_detail.js).
//
// Recognized data-* attributes (all optional):
//
//   data-mode              'assign' | 'filter'           default 'assign'
//   data-source-id         picker overlay routing key    default '' (auto)
//   data-initial           initial selected id/slug      default ''
//   data-placeholder       button label when nothing set default 'Select category...'
//   data-allow-empty       'true' | 'false'              default 'true'
//   data-empty-label       label for the empty option    default 'All categories'
//   data-categories-source JSONScript element id         default '' (use global)
//
// Outbound events (unchanged from the previous factory):
//
//   category-change   { id, slug, label }   — fired on selection.
//   open-category-picker — opens the global overlay; payload carries
//     mode, allowEmpty, emptyLabel, categories, sourceId so the overlay can
//     filter and route the resulting category-picked event back here.
//
// Inbound: window 'category-picked' { sourceId, id, slug, label } — when
// the global picker overlay (base.html) commits a selection that matches
// this picker's sourceId, we update selectedId and re-emit category-change.

document.addEventListener('alpine:init', function () {
  Alpine.data('categoryPicker', function () {
    return {
      open: false,
      search: '',
      selectedId: '',
      selectedLabel: '',
      categories: [],
      mode: 'assign',
      name: '',
      placeholder: 'Select category...',
      allowEmpty: true,
      emptyLabel: 'All categories',
      highlightIndex: -1,
      _bbSourceId: '',

      get flatList() {
        var items = [];
        if (this.allowEmpty) {
          items.push({ id: '', slug: '', label: this.emptyLabel, isParent: false, indent: false, icon: null, color: null, hidden: false });
        }
        for (var i = 0; i < this.categories.length; i++) {
          var cat = this.categories[i];
          // Skip the DB "uncategorized" category when allowEmpty provides its own empty option
          if (this.allowEmpty && cat.slug === 'uncategorized') continue;
          items.push({ id: cat.id, slug: cat.slug, label: cat.display_name, isParent: true, indent: false, icon: cat.icon, color: cat.color, hidden: cat.hidden || false });
          if (cat.children) {
            for (var j = 0; j < cat.children.length; j++) {
              var child = cat.children[j];
              // Children inherit the parent color when they don't define their own,
              // so colored dots render consistently regardless of category depth.
              items.push({ id: child.id, slug: child.slug, label: child.display_name, isParent: false, indent: true, icon: child.icon || cat.icon, color: child.color || cat.color, hidden: child.hidden || false, parentLabel: cat.display_name });
            }
          }
        }
        return items;
      },

      get filteredList() {
        var self = this;
        if (!this.search) return this.flatList.filter(function (i) { return !i.hidden || i.id === ''; });
        var q = this.search.toLowerCase();
        return this.flatList.filter(function (i) {
          if (i.hidden && i.id !== '') return false;
          if (i.id === '' && !self.allowEmpty) return false;
          return i.label.toLowerCase().indexOf(q) !== -1
            || (i.parentLabel && i.parentLabel.toLowerCase().indexOf(q) !== -1)
            || (i.slug && i.slug.toLowerCase().indexOf(q) !== -1);
        });
      },

      get value() {
        return this.mode === 'filter' ? this.selectedSlug : this.selectedId;
      },

      get selectedSlug() {
        var self = this;
        var item = this.flatList.find(function (i) {
          return self.mode === 'filter' ? i.slug === self.selectedId : i.id === self.selectedId;
        });
        return item ? item.slug : '';
      },

      get displayLabel() {
        var self = this;
        if (!this.selectedId) return this.allowEmpty ? this.emptyLabel : this.placeholder;
        var item = this.flatList.find(function (i) {
          return self.mode === 'filter' ? i.slug === self.selectedId : i.id === self.selectedId;
        });
        if (!item) return this.placeholder;
        return item.indent ? (item.parentLabel + ' › ' + item.label) : item.label;
      },

      get displayColor() {
        var self = this;
        if (!this.selectedId) return null;
        var item = this.flatList.find(function (i) {
          return self.mode === 'filter' ? i.slug === self.selectedId : i.id === self.selectedId;
        });
        return item ? item.color : null;
      },

      get displayIcon() {
        var self = this;
        if (!this.selectedId) return null;
        var item = this.flatList.find(function (i) {
          return self.mode === 'filter' ? i.slug === self.selectedId : i.id === self.selectedId;
        });
        return item ? item.icon : null;
      },

      itemValue: function (item) {
        return this.mode === 'filter' ? item.slug : item.id;
      },

      select: function (item) {
        this.selectedId = this.itemValue(item);
        this.open = false;
        this.search = '';
        this.highlightIndex = -1;
        this.$dispatch('category-change', { id: item.id, slug: item.slug, label: item.label });
        this.$nextTick(function () {
          if (typeof lucide !== 'undefined') lucide.createIcons({ nameAttr: 'data-lucide' });
        });
      },

      handleKeydown: function (e) {
        var list = this.filteredList;
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          this.highlightIndex = Math.min(this.highlightIndex + 1, list.length - 1);
          this.scrollToHighlighted();
        } else if (e.key === 'ArrowUp') {
          e.preventDefault();
          this.highlightIndex = Math.max(this.highlightIndex - 1, 0);
          this.scrollToHighlighted();
        } else if (e.key === 'Enter' && this.highlightIndex >= 0 && list[this.highlightIndex]) {
          e.preventDefault();
          this.select(list[this.highlightIndex]);
        } else if (e.key === 'Escape') {
          this.open = false;
          this.search = '';
          this.highlightIndex = -1;
        }
      },

      scrollToHighlighted: function () {
        var self = this;
        this.$nextTick(function () {
          var el = self.$refs.listbox && self.$refs.listbox.querySelector('[data-highlighted="true"]');
          if (el) el.scrollIntoView({ block: 'nearest' });
        });
      },

      get hasResults() {
        return this.filteredList.length > 0;
      },

      openPicker: function () {
        this.open = false;
        this.$dispatch('open-category-picker', {
          mode: this.mode,
          allowEmpty: this.allowEmpty,
          emptyLabel: this.emptyLabel,
          categories: this.categories,
          sourceId: this._bbSourceId,
        });
      },

      init: function () {
        var ds = this.$el.dataset || {};
        this.mode = ds.mode || 'assign';
        this.placeholder = ds.placeholder || 'Select category...';
        this.allowEmpty = ds.allowEmpty !== 'false';
        this.emptyLabel = ds.emptyLabel || 'All categories';
        this.selectedId = ds.initial || '';
        this._bbSourceId = ds.sourceId || '';

        // Categories: prefer an explicit JSONScript element when configured,
        // fall back to the page-seeded window.__bbCategories global.
        if (ds.categoriesSource) {
          var srcEl = document.getElementById(ds.categoriesSource);
          if (srcEl) {
            try {
              this.categories = JSON.parse(srcEl.textContent) || [];
            } catch (e) {
              console.error('categoryPicker: failed to parse #' + ds.categoriesSource, e);
              this.categories = [];
            }
          } else {
            this.categories = [];
          }
        } else {
          this.categories = window.__bbCategories || [];
        }

        // Ensure sourceId is set (prefer explicit value, fall back to data-picker-source on a parent).
        if (!this._bbSourceId) {
          var src = this.$el.closest('[data-picker-source]');
          this._bbSourceId = (src && src.dataset.pickerSource) || ('picker-' + Math.random().toString(36).slice(2, 8));
        }

        // Listen for category-picked responses from the global overlay (base.html).
        var self = this;
        window.addEventListener('category-picked', function (e) {
          if (e.detail.sourceId === self._bbSourceId) {
            self.selectedId = self.mode === 'filter' ? e.detail.slug : e.detail.id;
            self.$dispatch('category-change', { id: e.detail.id, slug: e.detail.slug, label: e.detail.label });
            self.$nextTick(function () {
              if (typeof lucide !== 'undefined') lucide.createIcons({ nameAttr: 'data-lucide' });
            });
          }
        });

        this.$nextTick(function () {
          if (typeof lucide !== 'undefined') lucide.createIcons({ nameAttr: 'data-lucide' });
        });
      },
    };
  });
});
