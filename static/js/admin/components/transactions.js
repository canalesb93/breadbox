// Transactions list page Alpine wiring for /transactions.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
//
// The page renders a few sibling x-data sub-blocks plus three Alpine stores
// (txView, bulk, txNav). All factories take no arguments; their initial
// state flows in via @templ.JSONScript and `data-*` attributes wired up in
// transactions.templ.
//
// One JSONScript payload:
//   #transactions-data -> { categories, allTags, filterTags, filterAnyTag }
//
// Module-level helpers (computeAppliedCounts, removeTag, updateRowTags,
// updateRowCategory, quickSetCategory, bbSetPerPage) are exposed on
// `window` so inline event handlers and AJAX-injected scripts can call
// them (the search fetch path inserts <script> tags via innerHTML and
// re-evals them — see _serverFetch in quickSearch).
//
// Alpine.data factories registered here:
//   - quickSearch  - the hybrid client/server quick search input.
//   - tagFilter    - the tag-filter chip group (multi-select + AND/OR).

// --- Bootstrap globals from the JSONScript payload ----------------------
//
// Seed window.__bbCategories / __bbAllTags / __bbTxFilterTags / __bbTxFilterAnyTag
// from the @templ.JSONScript("transactions-data", ...) tag emitted at the top
// of transactions.templ. The JSONScript element must precede this <script src>
// in the templ component so the IIFE finds the payload at parse time.
//
// These globals are read by:
//   - tx_row's inline categoryPicker (window.__bbCategories)
//   - the global tag picker in base.html (window.__bbAllTags)
//   - the inline tagFilter() factory below (window.__bbTxFilterTags / __bbTxFilterAnyTag)
//   - the bulk bar's category picker payload (window.__bbCategories)
(function seedGlobals() {
  var dataEl = document.getElementById('transactions-data');
  if (!dataEl) return;
  try {
    var payload = JSON.parse(dataEl.textContent) || {};
    window.__bbCategories = payload.categories || [];
    window.__bbAllTags = payload.allTags || [];
    window.__bbTxFilterTags = payload.filterTags || [];
    window.__bbTxFilterAnyTag = payload.filterAnyTag || [];
  } catch (e) {
    console.error('transactions: failed to parse #transactions-data', e);
    window.__bbCategories = window.__bbCategories || [];
    window.__bbAllTags = window.__bbAllTags || [];
    window.__bbTxFilterTags = window.__bbTxFilterTags || [];
    window.__bbTxFilterAnyTag = window.__bbTxFilterAnyTag || [];
  }
})();

/* ── Per-page selector ── */
function bbSetPerPage(val) {
  var url = new URL(window.location.href);
  url.searchParams.set('per_page', val);
  url.searchParams.delete('page'); // reset to page 1
  window.location.href = url.toString();
}

/*
 * Count tag memberships across a set of transaction IDs by reading each row's
 * rendered .bb-tag chips. Drives the picker's chip states: slug with count 0
 * is absent, count === ids.length is present, anything in between is mixed.
 *
 * We source the DOM instead of a server call because all selected rows are
 * already rendered on the page (bulk selection only covers the current view).
 */
function computeAppliedCounts(ids) {
  var counts = {};
  (ids || []).forEach(function(id) {
    var row = document.querySelector('.bb-tx-row[data-tx-id="' + CSS.escape(id) + '"]');
    if (!row) return;
    row.querySelectorAll('[data-tx-tag-container] .bb-tag').forEach(function(chip) {
      var slug = chip.getAttribute('title');
      if (!slug) return;
      counts[slug] = (counts[slug] || 0) + 1;
    });
  });
  return counts;
}

/* ── Remove tag from a transaction row ── */
function removeTag(el, txnId, slug, displayName) {
  fetch('/-/transactions/' + txnId + '/tags/' + encodeURIComponent(slug), { method: 'DELETE' })
  .then(function(r) { return r.json(); })
  .then(function(d) {
    if (d.ok || d.removed) {
      el.style.transition = 'opacity 150ms'; el.style.opacity = '0';
      setTimeout(function() { el.remove(); }, 160);
      window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message: displayName + ' removed', type:'success'}}));
    } else {
      window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message: d.error || 'Failed to remove tag', type:'error'}}));
    }
  });
}

/* ── In-place tag update on a row ──
 *
 * Adjusts the tag chips shown in a transaction row without a full page reload,
 * mirroring the category quick-set UX. Driven by the global tag cache
 * (`window.__bbAllTags`) for color/icon/display_name lookup.
 *
 *   updateRowTags(txId, { add: ['needs-review'], remove: ['suspicious'] })
 */
function updateRowTags(txId, changes) {
  if (!txId) return;
  var row = document.querySelector('.bb-tx-row[data-tx-id="' + CSS.escape(txId) + '"]');
  if (!row) return;
  var container = row.querySelector('[data-tx-tag-container]');
  if (!container) return;

  var add = (changes && changes.add) || [];
  var remove = (changes && changes.remove) || [];

  // Remove existing chips by slug (title attribute is the stable slug identifier
  // set by the tag-chip-sm partial).
  remove.forEach(function(slug) {
    var chip = container.querySelector('.bb-tag[title="' + CSS.escape(slug) + '"]');
    if (chip) chip.remove();
  });

  // Add chips, skipping any that are already present.
  add.forEach(function(slug) {
    if (container.querySelector('.bb-tag[title="' + CSS.escape(slug) + '"]')) return;
    var meta = (window.__bbAllTags || []).find(function(t) { return t.slug === slug; }) || {};
    var chip = document.createElement('span');
    chip.className = 'bb-tag bb-tag-sm';
    chip.title = slug;
    if (meta.color) chip.style.setProperty('--tag-color', meta.color);
    if (meta.icon) {
      var i = document.createElement('i');
      i.setAttribute('data-lucide', meta.icon);
      chip.appendChild(i);
    }
    var label = document.createElement('span');
    label.className = 'truncate';
    label.textContent = meta.display_name || slug;
    chip.appendChild(label);
    container.appendChild(chip);
  });

  // Toggle the leading separator so it only exists when there are tags.
  // The sep is desktop-only - on mobile the count pill below carries its own
  // leading middot.
  var sep = container.querySelector('[data-tx-tag-sep]');
  var chips = container.querySelectorAll('.bb-tag');
  var hasChips = chips.length > 0;
  if (hasChips && !sep) {
    sep = document.createElement('span');
    sep.className = 'text-base-content/20 hidden sm:inline';
    sep.setAttribute('data-tx-tag-sep', '');
    sep.textContent = '·';
    container.insertBefore(sep, container.firstChild);
  } else if (!hasChips && sep) {
    sep.remove();
  }

  // Sync the mobile-only tag count pill. Lives as a sibling of the container
  // so it can be `sm:hidden` without fighting the container's `contents`
  // display. Created on demand, removed when the row has no tags.
  var mobile = row.querySelector('[data-tx-tag-mobile]');
  if (hasChips) {
    if (!mobile) {
      mobile = document.createElement('span');
      mobile.className = 'sm:hidden inline-flex items-center gap-0.5 text-base-content/40 shrink-0';
      mobile.setAttribute('data-tx-tag-mobile', '');
      mobile.innerHTML = '<span class="text-base-content/20 mr-0.5">·</span>'
        + '<i data-lucide="tag" class="w-3 h-3"></i>'
        + '<span class="text-[0.65rem] tabular-nums font-medium" data-tx-tag-count></span>';
      container.parentNode.insertBefore(mobile, container.nextSibling);
    }
    var countEl = mobile.querySelector('[data-tx-tag-count]');
    if (countEl) countEl.textContent = chips.length;
    mobile.setAttribute('title', chips.length + ' tag' + (chips.length === 1 ? '' : 's'));
  } else if (mobile) {
    mobile.remove();
  }

  // Render any newly-added Lucide icons (container chips + mobile pill icon).
  if (typeof lucide !== 'undefined') lucide.createIcons({ nodes: [row] });
}

// updateRowCategory + _slugForCategoryId now live in
// static/js/admin/components/tx_row_helpers.js (loaded by transactions.templ
// and the other tx_row consumers). The local references below pick them up
// off `window`, so the rest of this file (bulk path) can keep calling
// `updateRowCategory(...)` unchanged.
var updateRowCategory = window.updateRowCategory;
var _slugForCategoryId = window._slugForCategoryId;

/* ── Single transaction category quick-set ──
 *
 * Posts the new category to the server, then mirrors the bulk path by
 * calling updateRowCategory() so the avatar icon, mobile compact label,
 * and any other category-derived row UI all update without a reload.
 * The inline cat-picker button itself is already reactive (selectedId →
 * displayLabel via Alpine), but the rest of the row was server-rendered
 * and stays stale until updateRowCategory rewrites it.
 */
function quickSetCategory(txId, categoryId) {
  if (!categoryId) {
    fetch('/-/transactions/' + txId + '/category', { method: 'DELETE' })
    .then(r => {
      if (r.ok || r.status === 204) {
        window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message:'Category reset', type:'success'}}));
        updateRowCategory(txId, '');
      }
    });
    return;
  }
  fetch('/-/transactions/' + txId + '/category', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({category_id: categoryId})
  })
  .then(r => {
    if (r.ok) {
      window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message:'Category updated', type:'success'}}));
      updateRowCategory(txId, _slugForCategoryId(categoryId));
    } else {
      r.json().then(d => window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message: d.error?.message || 'Failed', type:'error'}})));
    }
  });
}


// Expose helpers to window so inline event handlers, AJAX-injected scripts
// (eval'd inside _serverFetch), and the per-row inline cat-picker x-init
// watcher can call them.
window.bbSetPerPage = bbSetPerPage;
window.computeAppliedCounts = computeAppliedCounts;
window.removeTag = removeTag;
window.updateRowTags = updateRowTags;
// updateRowCategory is exposed by tx_row_helpers.js; quickSetCategory is
// the single-row variant defined above.
window.quickSetCategory = quickSetCategory;

document.addEventListener('alpine:init', function () {
  /* ── Tag filter (multi-select chips with AND/OR toggle) ── */
  Alpine.data('tagFilter', function () {
    return {
      availableTags: [],
      selectedSlugs: [],
      anyMode: false,

      init: function () {
        var initialSelected = window.__bbTxFilterTags || [];
        var initialAny = window.__bbTxFilterAnyTag || [];
        this.anyMode = initialAny && initialAny.length > 0;
        this.availableTags = window.__bbAllTags || [];
        this.selectedSlugs = this.anyMode ? initialAny.slice() : initialSelected.slice();
      },
      isSelected: function (slug) { return this.selectedSlugs.indexOf(slug) !== -1; },
      toggle: function (slug) {
        var idx = this.selectedSlugs.indexOf(slug);
        if (idx === -1) this.selectedSlugs.push(slug);
        else this.selectedSlugs.splice(idx, 1);
      },
    };
  });

  /* ── Hybrid Quick Search (Phase 1: instant client-side, Phase 2: server fetch) ── */
  Alpine.data('quickSearch', function () {
    return {
      query: '',
      searching: false,
      _abortCtrl: null,
      _fetchTimer: null,

      init: function () {
        this.query = new URLSearchParams(window.location.search).get('search') || '';
      },

      doSearch: function (val) {
        this.query = val;
        if (this._abortCtrl) this._abortCtrl.abort();
        clearTimeout(this._fetchTimer);

        if (!val.trim()) {
          this._restoreOriginal();
          return;
        }

        // Phase 1: Instant client-side filter on visible rows (no debounce)
        // Match server strategy: search name + merchant only (not account), all words must match
        var terms = val.toLowerCase().trim().split(/\s+/);
        document.querySelectorAll('.bb-tx-row').forEach(function (row) {
          var hay = (row.dataset.txName || '') + ' ' + (row.dataset.txMerchant || '');
          row.style.display = terms.every(function (t) { return hay.includes(t); }) ? '' : 'none';
          row.style.borderBottom = ''; // reset divide-y fix
        });
        document.querySelectorAll('[data-date-group]').forEach(function (group) {
          var visible = group.querySelectorAll('.bb-tx-row:not([style*="display: none"])');
          var count = visible.length;
          group.style.display = count ? '' : 'none';
          // Fix divide-y: last visible row gets a spurious border-bottom when hidden siblings follow
          if (count > 0) visible[count - 1].style.borderBottom = '0px';
          // Update transaction count
          var countEl = group.querySelector('[data-txn-count]');
          if (countEl) countEl.textContent = count + ' txn' + (count !== 1 ? 's' : '');
          // Recalculate spending/income from visible rows
          var spending = 0, income = 0;
          visible.forEach(function (row) {
            var amt = parseFloat(row.dataset.txAmount) || 0;
            if (amt > 0) spending += amt; else income += Math.abs(amt);
          });
          var spendEl = group.querySelector('[data-day-spending]');
          if (spendEl) {
            spendEl.textContent = spending > 0 ? '$' + spending.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : '';
            spendEl.style.display = spending > 0 ? '' : 'none';
          }
          var incomeEl = group.querySelector('[data-day-income]');
          if (incomeEl) {
            incomeEl.textContent = income > 0 ? '-$' + income.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : '';
            incomeEl.style.display = income > 0 ? '' : 'none';
          }
        });

        // Append skeleton rows to hint that server results are loading
        var skeletonRow = '<div class="flex items-center gap-3 px-4 py-3 animate-pulse">'
          + '<div class="w-5 h-5 bg-base-300/30 rounded shrink-0"></div>'
          + '<div class="w-9 h-9 bg-base-300/30 rounded-xl shrink-0"></div>'
          + '<div class="flex-1 space-y-1.5"><div class="h-3 bg-base-300/30 rounded w-2/5"></div><div class="h-2.5 bg-base-300/20 rounded w-1/4"></div></div>'
          + '<div class="h-3 bg-base-300/30 rounded w-20 hidden sm:block"></div>'
          + '<div class="h-3 bg-base-300/20 rounded w-14 hidden sm:block"></div>'
          + '<div class="h-3.5 bg-base-300/30 rounded w-16"></div>'
          + '</div>';
        var skeletonEl = document.getElementById('bb-search-skeleton');
        if (skeletonEl) skeletonEl.remove();
        var skeleton = document.createElement('div');
        skeleton.id = 'bb-search-skeleton';
        var mode = (window.Alpine && Alpine.store('txView')) ? Alpine.store('txView').mode : 'grouped';
        if (mode === 'grouped') {
          var groups = '';
          for (var g = 0; g < 3; g++) {
            groups += '<div class="animate-pulse">'
              + '<div class="flex items-center justify-between px-2 pt-4 pb-2">'
              + '<div class="flex items-center gap-3"><div class="h-3 bg-base-300/30 rounded w-24"></div><div class="h-2.5 bg-base-300/20 rounded w-12"></div></div>'
              + '<div class="h-3 bg-base-300/20 rounded w-14"></div>'
              + '</div>'
              + '<div class="bb-card overflow-hidden divide-y divide-base-300/30">' + skeletonRow + '</div>'
              + '</div>';
          }
          skeleton.innerHTML = '<div class="space-y-1">' + groups + '</div>';
        } else {
          var rows = '';
          for (var i = 0; i < 3; i++) {
            rows += skeletonRow;
            if (i < 2) rows += '<div class="border-t border-base-300/20"></div>';
          }
          skeleton.innerHTML = '<div class="bb-card overflow-hidden"><div class="divide-y divide-base-300/30">' + rows + '</div></div>';
        }
        var resultsEl = document.getElementById('bb-tx-results');
        var paginatorEl = document.getElementById('bb-tx-paginator');
        if (paginatorEl) paginatorEl.style.display = 'none';
        if (resultsEl) {
          if (paginatorEl) resultsEl.insertBefore(skeleton, paginatorEl);
          else resultsEl.appendChild(skeleton);
        }

        // Phase 2: Server fetch for full results (debounced 500ms)
        var self = this;
        this._fetchTimer = setTimeout(function () { self._serverFetch(val); }, 500);
      },

      _serverFetch: function (val) {
        this.searching = true;
        this._abortCtrl = new AbortController();
        var searchUrl = new URL(window.location.origin + '/transactions/search');
        // Preserve existing filters
        new URLSearchParams(window.location.search).forEach(function (v, k) {
          if (k !== 'search' && k !== 'search_mode' && k !== 'page') searchUrl.searchParams.set(k, v);
        });
        searchUrl.searchParams.set('search', val);
        searchUrl.searchParams.set('search_mode', 'words');
        searchUrl.searchParams.set('page', '1');

        var self = this;
        fetch(searchUrl.toString(), { signal: this._abortCtrl.signal })
          .then(function (r) { return r.text(); })
          .then(function (html) {
            var results = document.getElementById('bb-tx-results');
            if (results) {
              // Check if response has any transaction rows
              var hasResults = html.indexOf('bb-tx-row') !== -1;
              if (hasResults) {
                results.innerHTML = html.replace(/bb-stagger/g, '');
                results.querySelectorAll('script').forEach(function (s) { eval(s.textContent); });
                Alpine.initTree(results);
                if (window.lucide) lucide.createIcons();
              } else {
                results.innerHTML = '<div class="bb-card">'
                  + '<div class="flex flex-col items-center text-center py-12 px-6">'
                  + '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="text-base-content/15 mb-3"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/><path d="m15 8-6 6"/></svg>'
                  + '<h3 class="text-sm font-semibold text-base-content/60 mb-1">No results found</h3>'
                  + '<p class="text-xs text-base-content/40">No transactions match "<strong>' + val.replace(/</g, '&lt;') + '</strong>". Try a different search term or check your filters.</p>'
                  + '</div></div>';
              }
              results.style.opacity = '1';
              results.style.pointerEvents = '';
            }
            // Update total badge from response
            var m = html.match(/Showing\s+\d+/);
            var totalBadge = document.querySelector('.badge.badge-ghost.badge-sm.tabular-nums');
            if (totalBadge) {
              var countMatch = html.match(/ of (\d+)/);
              if (countMatch) totalBadge.textContent = countMatch[1] + ' total';
            }
            // Update URL without reload
            var displayUrl = new URL(window.location);
            displayUrl.searchParams.set('search', val);
            displayUrl.searchParams.set('search_mode', 'words');
            displayUrl.searchParams.set('page', '1');
            window.history.replaceState({}, '', displayUrl.toString());
            self.searching = false;
          })
          .catch(function (e) {
            if (e.name !== 'AbortError') self.searching = false;
          });
      },

      _restoreOriginal: function () {
        if (this._abortCtrl) this._abortCtrl.abort();
        clearTimeout(this._fetchTimer);
        this.searching = false;
        // Remove any skeleton and restore pagination
        var skel = document.getElementById('bb-search-skeleton');
        if (skel) skel.remove();
        var pag = document.getElementById('bb-tx-paginator');
        if (pag) pag.style.display = '';
        // Show all client-side hidden rows and restore counts
        document.querySelectorAll('.bb-tx-row').forEach(function (r) { r.style.display = ''; r.style.borderBottom = ''; });
        document.querySelectorAll('[data-date-group]').forEach(function (g) {
          g.style.display = '';
          var countEl = g.querySelector('[data-txn-count]');
          if (countEl) {
            var orig = parseInt(countEl.dataset.txnCount, 10);
            countEl.textContent = orig + ' txn' + (orig !== 1 ? 's' : '');
          }
          var spendEl = g.querySelector('[data-day-spending]');
          if (spendEl) {
            var origSpend = parseFloat(spendEl.dataset.daySpending) || 0;
            spendEl.textContent = origSpend > 0 ? '$' + origSpend.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : '';
            spendEl.style.display = origSpend > 0 ? '' : 'none';
          }
          var incomeEl = g.querySelector('[data-day-income]');
          if (incomeEl) {
            var origIncome = parseFloat(incomeEl.dataset.dayIncome) || 0;
            incomeEl.textContent = origIncome > 0 ? '-$' + origIncome.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : '';
            incomeEl.style.display = origIncome > 0 ? '' : 'none';
          }
        });
        // If URL has search params, fetch original results without reload
        if (new URLSearchParams(window.location.search).has('search')) {
          var url = new URL(window.location.origin + '/transactions/search');
          new URLSearchParams(window.location.search).forEach(function (v, k) {
            if (k !== 'search' && k !== 'search_mode' && k !== 'page') url.searchParams.set(k, v);
          });
          url.searchParams.set('page', '1');
          var self = this;
          self.searching = true;
          fetch(url.toString())
            .then(function (r) { return r.text(); })
            .then(function (html) {
              var results = document.getElementById('bb-tx-results');
              if (results) {
                results.innerHTML = html.replace(/bb-stagger/g, '');
                results.querySelectorAll('script').forEach(function (s) { eval(s.textContent); });
                Alpine.initTree(results);
                if (window.lucide) lucide.createIcons();
              }
              var displayUrl = new URL(window.location);
              displayUrl.searchParams.delete('search');
              displayUrl.searchParams.delete('search_mode');
              displayUrl.searchParams.set('page', '1');
              window.history.replaceState({}, '', displayUrl.toString());
              self.searching = false;
            })
            .catch(function () { self.searching = false; });
        }
      },

      clear: function () {
        this.query = '';
        if (this._abortCtrl) this._abortCtrl.abort();
        this.$refs.searchInput.value = '';
        this._restoreOriginal();
        var input = this.$refs.searchInput;
        setTimeout(function () { input.focus(); }, 50);
      }
    };
  });

  /* ── View Toggle (Alpine store for shared state) ── */
  Alpine.store('txView', {
    mode: localStorage.getItem('bb-tx-view') || 'grouped',
    setMode(m) {
      this.mode = m;
      localStorage.setItem('bb-tx-view', m);
      // Re-render Lucide icons after Alpine updates DOM
      setTimeout(function() {
        if (typeof lucide !== 'undefined') lucide.createIcons();
      }, 50);
    }
  });

  /*
   * Bulk selection store. Selection state is stored in a Set for O(1) membership
   * tests and the DOM is updated imperatively - no per-row Alpine reactivity.
   * With ~50 rows on screen, a reactive `isIn(id)` binding on every row caused
   * the checkbox draw animation to compete with Alpine's reconciliation.
   */
  Alpine.store('bulk', {
    selSet: new Set(),
    get sel() { return Array.from(this.selSet); },
    set sel(v) { this.selSet = new Set(v || []); this._syncAllRows(); },
    selecting: false,
    processing: false,

    toggleMode() {
      this.selecting = !this.selecting;
      if (!this.selecting) { this.selSet = new Set(); this._syncAllRows(); }
      this._updateBar();
      this._updateRowVisibility();
    },

    _updateBar() {
      var bar = document.getElementById('bb-bulk-bar');
      if (!bar) return;
      var show = this.selecting;
      bar.style.opacity = show ? '1' : '0';
      bar.style.transform = 'translateX(-50%) translateY(' + (show ? '0' : '1rem') + ')';
      bar.style.pointerEvents = show ? 'auto' : 'none';
      var countEl = document.getElementById('bb-bulk-count');
      if (countEl) countEl.textContent = this.selSet.size;
    },

    // Show/hide the per-row checkbox labels when selection mode toggles.
    // Done once globally instead of 50 reactive x-show bindings.
    // Set display explicitly (not '') so inline style wins over Tailwind `.flex`.
    // Also toggles a body-level class so CSS can hide the per-row avatar slot
    // at mobile breakpoints (the checkbox reuses that slot's width instead of
    // appending to it, which was truncating merchant names - see #593).
    _updateRowVisibility() {
      var on = this.selecting;
      document.querySelectorAll('.bb-tx-checkbox').forEach(function(el) {
        el.style.display = on ? 'flex' : 'none';
      });
      document.body.classList.toggle('bb-tx-selecting', on);
    },

    // Update a single row's checkbox + selected attribute without touching others.
    _syncRow(id) {
      var row = document.querySelector('.bb-tx-row[data-tx-id="' + CSS.escape(id) + '"]');
      if (!row) return;
      var on = this.selSet.has(id);
      if (on) row.setAttribute('data-tx-selected', ''); else row.removeAttribute('data-tx-selected');
      var cb = row.querySelector('[data-tx-checkbox]');
      if (cb) cb.checked = on;
    },

    // Resync every row (used after bulk operations like selectAll / toggleMode).
    _syncAllRows() {
      var self = this;
      document.querySelectorAll('.bb-tx-row').forEach(function(row) {
        var id = row.dataset.txId;
        var on = self.selSet.has(id);
        if (on) row.setAttribute('data-tx-selected', ''); else row.removeAttribute('data-tx-selected');
        var cb = row.querySelector('[data-tx-checkbox]');
        if (cb) cb.checked = on;
      });
      // Update the select-all indeterminate/checked state too.
      var all = document.querySelector('[data-tx-selectall-checkbox]');
      if (all) {
        var total = (window.__bbTxMeta || []).length;
        all.checked = self.selSet.size === total && total > 0;
        all.indeterminate = self.selSet.size > 0 && self.selSet.size < total;
      }
    },

    isIn(id) {
      return this.selSet.has(id);
    },

    toggle(id) {
      if (this.selSet.has(id)) this.selSet.delete(id);
      else this.selSet.add(id);
      this._syncRow(id);
      this._updateBar();
    },

    toggleAll() {
      var total = (window.__bbTxMeta || []).length;
      if (this.selSet.size === total) {
        this.selSet = new Set();
      } else {
        this.selSet = new Set((window.__bbTxMeta || []).map(function(t) { return t.id; }));
      }
      this._syncAllRows();
      this._updateBar();
    },

    clear() {
      this.selSet = new Set();
      this._syncAllRows();
      this._updateBar();
    },


    categorize(categoryId) {
      if (!categoryId || this.sel.length === 0) return;
      var self = this;
      self.processing = true;
      var ids = self.sel.slice();
      // Look up the slug for the picked id so updateRowCategory can resolve
      // color/icon/name from window.__bbCategories.
      var slug = '';
      var cats = window.__bbCategories || [];
      outer: for (var i = 0; i < cats.length; i++) {
        if (cats[i].id === categoryId) { slug = cats[i].slug; break; }
        if (cats[i].children) {
          for (var j = 0; j < cats[i].children.length; j++) {
            if (cats[i].children[j].id === categoryId) { slug = cats[i].children[j].slug; break outer; }
          }
        }
      }
      // Optimistic in-place update for every selected row.
      if (slug) ids.forEach(function(id) { updateRowCategory(id, slug); });

      var items = ids.map(function(id) { return { transaction_id: id, category_id: categoryId }; });
      fetch('/-/transactions/batch-categorize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ items: items })
      })
      // Catch JSON parse failures (non-JSON 5xx from a proxy, etc) here so
      // they reach the outer .catch - without this guard the throw bypasses
      // the catch and `processing` stays true forever, silently wedging the
      // per-row category-picker $watch guard for the rest of the session.
      .then(function(r) { return r.json().then(function(d) { return {ok: r.ok, body: d}; }); })
      .then(function(res) {
        self.processing = false;
        var data = res.body || {};
        if (res.ok && data.succeeded > 0) {
          var msg = data.succeeded + ' transaction' + (data.succeeded === 1 ? '' : 's') + ' categorized' + (data.failed > 0 ? ' (' + data.failed + ' failed)' : '');
          window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message: msg, type: data.failed > 0 ? 'warning' : 'success'}}));
        } else {
          window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message:'Failed to categorize transactions', type:'error'}}));
        }
      })
      .catch(function() {
        self.processing = false;
        window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message:'Network error', type:'error'}}));
      });
    },

    /* runBatchUpdate dispatches a batch update_transactions call applying the
     * same op (sans transaction_id) to every listed transaction. The op may
     * include category_slug, tags_to_add, tags_to_remove, and comment.
     *
     * `ids` defaults to the current bulk selection, but can be overridden so
     * the same code path handles single-transaction tag diffs (row shortcut,
     * detail page) as bulk tag diffs - keeping optimistic UI + chunking in
     * one place.
     *
     * All visible row state (tags, category) is updated in place - no reload.
     * Comments are background-only (not shown on this list). The selection
     * stays intact so the user can chain more bulk ops on the same set. */
    runBatchUpdate(op, ids) {
      ids = ids && ids.length ? ids.slice() : this.sel.slice();
      if (ids.length === 0) return;
      var self = this;
      // The /-/transactions/batch-update endpoint caps each call at 50 ops.
      var CHUNK = 50;
      var chunks = [];
      for (var i = 0; i < ids.length; i += CHUNK) {
        chunks.push(ids.slice(i, i + CHUNK));
      }
      self.processing = true;
      var totalSucceeded = 0, totalFailed = 0;

      // Optimistic in-place updates. Rolled back per-row on API failure
      // would be ideal, but the batch endpoint reports succeeded/failed
      // counts without per-row mapping - and the user already sees the
      // toast count, so partial-rollback complexity isn't worth it.
      if (op.tags_to_add || op.tags_to_remove) {
        var addSlugs = (op.tags_to_add || []).map(function(t) { return t.slug; });
        var removeSlugs = (op.tags_to_remove || []).map(function(t) { return t.slug; });
        ids.forEach(function(id) { updateRowTags(id, {add: addSlugs, remove: removeSlugs}); });
      }
      if (op.category_slug) {
        ids.forEach(function(id) { updateRowCategory(id, op.category_slug); });
      }
      // op.comment has no visible row state to update.

      function runOne(idx) {
        if (idx >= chunks.length) {
          self.processing = false;
          var msg = totalSucceeded + ' transaction' + (totalSucceeded === 1 ? '' : 's') + ' updated' + (totalFailed > 0 ? ' (' + totalFailed + ' failed)' : '');
          window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message: msg, type: totalFailed > 0 ? 'warning' : 'success'}}));
          return;
        }
        var ops = chunks[idx].map(function(id) {
          var entry = {transaction_id: id};
          if (op.category_slug) entry.category_slug = op.category_slug;
          if (op.tags_to_add) entry.tags_to_add = op.tags_to_add;
          if (op.tags_to_remove) entry.tags_to_remove = op.tags_to_remove;
          if (op.comment) entry.comment = op.comment;
          return entry;
        });
        fetch('/-/transactions/batch-update', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({operations: ops, on_error: 'continue'}),
        })
        .then(function(r) { return r.json().then(function(d) { return {ok: r.ok || r.status === 422, body: d}; }); })
        .then(function(r) {
          if (r.body && typeof r.body.succeeded === 'number') totalSucceeded += r.body.succeeded;
          if (r.body && typeof r.body.failed === 'number') totalFailed += r.body.failed;
          runOne(idx + 1);
        })
        .catch(function() {
          totalFailed += chunks[idx].length;
          runOne(idx + 1);
        });
      }
      runOne(0);
    }
  });

  /* ── Alpine Store: Keyboard Navigation ── */
  Alpine.store('txNav', {
    focusedIdx: -1,

    _getRows() {
      return Array.from(document.querySelectorAll('.bb-tx-row:not([style*="display: none"])'));
    },

    _updateFocus(oldIdx) {
      var rows = this._getRows();
      if (oldIdx >= 0 && rows[oldIdx]) rows[oldIdx].classList.remove('bb-tx-row--focused');
      if (this.focusedIdx >= 0 && rows[this.focusedIdx]) {
        rows[this.focusedIdx].classList.add('bb-tx-row--focused');
        rows[this.focusedIdx].scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      }
    },

    next() {
      var rows = this._getRows();
      if (!rows.length) return;
      var old = this.focusedIdx;
      this.focusedIdx = Math.min(this.focusedIdx + 1, rows.length - 1);
      this._updateFocus(old);
    },

    prev() {
      var rows = this._getRows();
      if (!rows.length) return;
      var old = this.focusedIdx;
      this.focusedIdx = Math.max(this.focusedIdx - 1, 0);
      this._updateFocus(old);
    },

    openDetail() {
      var row = this._getRows()[this.focusedIdx];
      if (!row) return;
      var link = row.querySelector('a[href*="/transactions/"]');
      if (link) window.location.href = link.href;
    },

    openCategorize() {
      // If there's a bulk selection, the shortcut should act on the whole set.
      // Route through the bulk-categorize button so the same picker + commit
      // path runs (keeps sourceId + listener wiring in one place).
      var bulk = Alpine.store('bulk');
      if (bulk.selecting && bulk.sel.length > 0) {
        var btn = document.getElementById('bb-bulk-categorize');
        if (btn) btn.click();
        return;
      }
      var row = this._getRows()[this.focusedIdx];
      if (!row) return;
      var inline = row.querySelector('.bb-cat-picker button');
      if (inline) inline.click();
    },

    openTagPicker() {
      // Bulk selection wins - 't' on any focused row should edit tags for the
      // whole selection, not just the focused one.
      var bulk = Alpine.store('bulk');
      if (bulk.selecting && bulk.sel.length > 0) {
        var btn = document.getElementById('bb-bulk-tag');
        if (btn) btn.click();
        return;
      }
      var row = this._getRows()[this.focusedIdx];
      if (!row) return;
      // Read the id from data-tx-id (set on every row), not the link href -
      // splitting href on '/' would break silently if the URL ever grew a
      // query string or fragment.
      var id = row.dataset.txId;
      if (!id) return;
      window.dispatchEvent(new CustomEvent('open-tag-picker', {
        detail: {
          sourceId: 'txrow-' + id,
          transactionIds: [id],
          txCount: 1,
          appliedCounts: computeAppliedCounts([id]),
          availableTags: window.__bbAllTags || [],
        }
      }));
    },

    toggleExpand() {
      var row = this._getRows()[this.focusedIdx];
      if (!row) return;
      var main = row.querySelector('.cursor-pointer');
      if (main) main.click();
    },

    toggleSelect() {
      var row = this._getRows()[this.focusedIdx];
      if (!row) return;
      var id = row.dataset.txId;
      if (!id) return;
      if (!Alpine.store('bulk').selecting) Alpine.store('bulk').toggleMode();
      Alpine.store('bulk').toggle(id);
    },

    // Jump the keyboard focus ring onto the clicked row so j/k continues from there.
    focusRow(row) {
      var rows = this._getRows();
      var idx = rows.indexOf(row);
      if (idx === -1) return;
      var old = this.focusedIdx;
      this.focusedIdx = idx;
      // Don't auto-scroll on click - the user is already looking at this row.
      if (old >= 0 && rows[old]) rows[old].classList.remove('bb-tx-row--focused');
      rows[idx].classList.add('bb-tx-row--focused');
    },

    clearFocus() {
      var rows = this._getRows();
      if (this.focusedIdx >= 0 && rows[this.focusedIdx]) {
        rows[this.focusedIdx].classList.remove('bb-tx-row--focused');
      }
      this.focusedIdx = -1;
    }
  });

  // Priming happens in the DOMContentLoaded handler below, after the bulk bar
  // has been appended - `alpine:init` runs before DOMContentLoaded handlers,
  // so _updateBar() would otherwise early-return (bar element not yet in DOM)
  // and the bar would stay opacity:0 even when selection was restored.

  // When the user clicks a row to expand/collapse, move the j/k focus ring to
  // that row so keyboard nav picks up from where the user was looking.
  document.addEventListener('click', function(e) {
    // Ignore clicks on interactive descendants (checkboxes, buttons, pickers, links)
    // - those have their own handlers and we don't want to yank focus out from under them.
    if (e.target.closest('button, a, input, label, .bb-cat-picker, .bb-tagpicker-dialog, .bb-catpicker-dialog')) return;
    var row = e.target.closest('.bb-tx-row');
    if (!row) return;
    var nav = Alpine.store('txNav');
    if (nav) nav.focusRow(row);
  });

  // Register transactions-scoped shortcuts against the global $store.shortcuts
  // registry. The base.html dispatcher reads from this registry, so no inline
  // keydown listener is needed here - and the help modal + cmdk can surface
  // these bindings automatically on this page.
  //
  // Scope is activated when the transactions page runs its x-init
  // ($store.shortcuts.setScope('transactions')); all specs below declare
  // scope: 'transactions' so they only fire there.
  function txPickerOpen() {
    // Category overlay sets inline display:block when open.
    if (document.querySelector('.bb-cat-overlay[style*="block"]')) return true;
    function dialogShown(sel) {
      var d = document.querySelector(sel);
      if (!d || !d.parentElement) return false;
      return window.getComputedStyle(d.parentElement).display !== 'none';
    }
    return dialogShown('.bb-catpicker-dialog') || dialogShown('.bb-tagpicker-dialog');
  }
  var txHasFocus = function() { return Alpine.store('txNav').focusedIdx >= 0; };
  var txNotInPicker = function() { return !txPickerOpen(); };
  var txFocusedNotInPicker = function() { return txHasFocus() && txNotInPicker(); };
  var reg = Alpine.store('shortcuts');

  reg.register({ id: 'transactions.next', keys: 'j', description: 'Move focus down',
    group: 'Navigation', scope: 'transactions', when: txNotInPicker,
    action: function() { Alpine.store('txNav').next(); } });
  reg.register({ id: 'transactions.prev', keys: 'k', description: 'Move focus up',
    group: 'Navigation', scope: 'transactions', when: txNotInPicker,
    action: function() { Alpine.store('txNav').prev(); } });
  reg.register({ id: 'transactions.open', keys: 'Enter', description: 'Open focused transaction',
    group: 'Actions', scope: 'transactions', when: txFocusedNotInPicker,
    action: function() { Alpine.store('txNav').openDetail(); } });
  reg.register({ id: 'transactions.categorize', keys: 'c', description: 'Categorize',
    group: 'Actions', scope: 'transactions', when: txFocusedNotInPicker,
    action: function() { Alpine.store('txNav').openCategorize(); } });
  reg.register({ id: 'transactions.tag', keys: 't', description: 'Tag',
    group: 'Actions', scope: 'transactions', when: txFocusedNotInPicker,
    action: function() { Alpine.store('txNav').openTagPicker(); } });
  reg.register({ id: 'transactions.expand', keys: 'e', description: 'Expand / collapse row',
    group: 'Actions', scope: 'transactions', when: txFocusedNotInPicker,
    action: function() { Alpine.store('txNav').toggleExpand(); } });
  reg.register({ id: 'transactions.select', keys: 'x', description: 'Toggle selection',
    group: 'Selection', scope: 'transactions', when: txFocusedNotInPicker,
    action: function() { Alpine.store('txNav').toggleSelect(); } });
  reg.register({ id: 'transactions.focus-search', keys: '/',
    description: 'Focus search',
    group: 'Actions', scope: 'transactions', when: txNotInPicker,
    action: function() {
      var input = document.getElementById('bb-quick-search');
      if (input) { input.focus(); input.select && input.select(); }
    } });
  // Escape priority: deselect bulk -> exit select mode -> clear j/k focus.
  // Bulk state wins over the focus ring because it's the heavier piece of
  // state the user is likely to want to back out of first; clearing focus
  // is quietest and goes last. Registered page-scoped so it takes precedence
  // over any future global Esc binding.
  reg.register({ id: 'transactions.escape', keys: 'Escape',
    description: 'Clear selection / focus',
    group: 'Navigation', scope: 'transactions', when: txNotInPicker,
    visible: false,
    action: function() {
      var bulk = Alpine.store('bulk');
      var nav = Alpine.store('txNav');
      if (bulk.selecting && bulk.sel.length > 0) bulk.clear();
      else if (bulk.selecting) bulk.toggleMode();
      else if (nav.focusedIdx >= 0) nav.clearFocus();
    } });

});

/* ── Floating Bulk Action Bar (injected into <body> to avoid parent transform issues) ── */
document.addEventListener('DOMContentLoaded', function() {
  var bar = document.createElement('div');
  bar.id = 'bb-bulk-bar';
  bar.style.cssText = 'position:fixed;bottom:1.5rem;left:50%;z-index:40;opacity:0;transform:translateX(-50%) translateY(1rem);pointer-events:none;transition:opacity 0.2s ease,transform 0.2s ease';
  bar.innerHTML = '<div class="flex items-center gap-2 bg-base-100 border border-base-300 rounded-2xl shadow-lg px-4 py-2.5 max-w-[calc(100vw-2rem)]">'
    + '<button type="button" id="bb-bulk-count-toggle" class="btn btn-ghost btn-sm rounded-xl gap-1.5" title="Select all / clear">'
    +   '<span id="bb-bulk-count" class="font-bold tabular-nums">0</span>'
    +   '<span class="hidden sm:inline text-base-content/60 font-normal">selected</span>'
    + '</button>'
    + '<div class="w-px h-6 bg-base-300"></div>'
    + '<button type="button" id="bb-bulk-comment" class="btn btn-ghost btn-sm rounded-xl gap-2" title="Add comment">'
    +   '<i data-lucide="message-square" class="w-3.5 h-3.5"></i>'
    +   '<span class="hidden sm:inline">Comment</span>'
    + '</button>'
    + '<button type="button" id="bb-bulk-tag" class="btn btn-ghost btn-sm rounded-xl gap-2" title="Tag selected">'
    +   '<i data-lucide="tag" class="w-3.5 h-3.5"></i>'
    +   '<span class="hidden sm:inline">Tag</span>'
    +   '<span class="hidden sm:inline ml-1 text-[10px] opacity-50 border border-current/30 rounded px-1 leading-none py-0.5">T</span>'
    + '</button>'
    + '<button type="button" id="bb-bulk-categorize" class="btn btn-primary btn-sm rounded-xl gap-2" title="Categorize selected">'
    +   '<i data-lucide="tags" class="w-3.5 h-3.5"></i>'
    +   '<span class="hidden sm:inline">Categorize</span>'
    +   '<span class="hidden sm:inline ml-1 text-[10px] opacity-60 border border-current/30 rounded px-1 leading-none py-0.5">C</span>'
    + '</button>'
    + '<button type="button" id="bb-bulk-clear" class="btn btn-ghost btn-sm btn-square rounded-xl" title="Exit select mode">'
    +   '<i data-lucide="x" class="w-4 h-4"></i>'
    + '</button>'
    + '</div>';
  document.body.appendChild(bar);
  if (typeof lucide !== 'undefined') lucide.createIcons({ nameAttr: 'data-lucide', attrs: {}, namePrefix: '' });

  // Inject the bulk comment modal. Tag add/remove both go through the global
  // tag picker (see bb-bulk-tag click handler below), so no tag modal is needed.
  var modal = document.createElement('div');
  modal.id = 'bb-bulk-modal-root';
  modal.innerHTML =
    '<dialog id="bb-bulk-comment-modal" class="modal modal-bottom sm:modal-middle">'
    + '<div class="modal-box rounded-xl max-w-md">'
    + '<h3 class="text-lg font-bold">Add comment to selected</h3>'
    + '<p class="text-sm text-base-content/50 mt-1">Adds the same comment annotation to <span id="bb-comment-count" class="font-semibold">0</span> selected transactions.</p>'
    + '<div class="space-y-3 mt-4">'
    + '<textarea id="bb-comment-content" class="textarea textarea-bordered w-full rounded-xl" rows="4" placeholder="Markdown supported"></textarea>'
    + '</div>'
    + '<div class="modal-action">'
    + '<button type="button" class="btn btn-ghost rounded-xl" onclick="document.getElementById(\'bb-bulk-comment-modal\').close()">Cancel</button>'
    + '<button type="button" id="bb-comment-confirm" class="btn btn-primary rounded-xl">Add comment</button>'
    + '</div>'
    + '</div>'
    + '<form method="dialog" class="modal-backdrop"><button>close</button></form>'
    + '</dialog>';
  document.body.appendChild(modal);

  document.getElementById('bb-bulk-count-toggle').addEventListener('click', function() {
    // Gmail-style: clicking the count toggles between "select all on this page"
    // and "clear all". Gives the user a one-click path to each extreme without
    // hunting for a menu.
    Alpine.store('bulk').toggleAll();
  });
  document.getElementById('bb-bulk-categorize').addEventListener('click', function() {
    window.dispatchEvent(new CustomEvent('open-category-picker', {
      detail: {
        mode: 'assign',
        allowEmpty: false,
        emptyLabel: '',
        categories: window.__bbCategories || [],
        sourceId: 'bulk-categorize'
      }
    }));
  });

  // Bulk "Tag" button - opens the global tag picker for every selected row.
  // The picker is always multi + always diff-based, so the same contract is
  // used for 1 selected row and N selected rows.
  document.getElementById('bb-bulk-tag').addEventListener('click', function() {
    var ids = Alpine.store('bulk').sel.slice();
    window.dispatchEvent(new CustomEvent('open-tag-picker', {
      detail: {
        sourceId: 'bulk-tag',
        transactionIds: ids,
        txCount: ids.length,
        appliedCounts: computeAppliedCounts(ids),
        availableTags: window.__bbAllTags || [],
      }
    }));
  });

  document.getElementById('bb-bulk-comment').addEventListener('click', function() {
    document.getElementById('bb-comment-count').textContent = Alpine.store('bulk').sel.length;
    document.getElementById('bb-comment-content').value = '';
    document.getElementById('bb-bulk-comment-modal').showModal();
  });
  document.getElementById('bb-comment-confirm').addEventListener('click', function() {
    var content = (document.getElementById('bb-comment-content').value || '').trim();
    if (!content) {
      window.dispatchEvent(new CustomEvent('bb-toast', {detail:{message:'Comment cannot be empty', type:'error'}}));
      return;
    }
    Alpine.store('bulk').runBatchUpdate({comment: content});
    document.getElementById('bb-bulk-comment-modal').close();
  });
  document.getElementById('bb-bulk-clear').addEventListener('click', function() {
    Alpine.store('bulk').toggleMode();
  });

  window.addEventListener('category-picked', function(e) {
    if (e.detail.sourceId === 'bulk-categorize' && e.detail.id) {
      Alpine.store('bulk').categorize(e.detail.id);
    }
  });

  // Unified tag-picker commit listener. Every caller that opens the picker
  // (bulk bar, row 't' shortcut, tx detail) receives the same diff event here
  // and routes through runBatchUpdate - optimistic UI + chunking + toasts all
  // handled in one place.
  window.addEventListener('tag-selection-commit', function(e) {
    var d = e && e.detail;
    if (!d) return;
    // The transaction detail page commits its own diff (it needs to reload for
    // the activity timeline). Let that handler claim its sourceId.
    if (d.sourceId === 'txd-tag') return;
    var ids = (d.transactionIds && d.transactionIds.length) ? d.transactionIds : Alpine.store('bulk').sel;
    if (!ids || ids.length === 0) return;
    var op = {};
    if (d.adds && d.adds.length) op.tags_to_add = d.adds.map(function(s) { return {slug: s, note: ''}; });
    if (d.removes && d.removes.length) op.tags_to_remove = d.removes.map(function(s) { return {slug: s, note: ''}; });
    if (!op.tags_to_add && !op.tags_to_remove) return;
    Alpine.store('bulk').runBatchUpdate(op, ids);
  });

  // Sync the bulk store's DOM state once the bar is actually in the DOM.
  // alpine:init fires earlier but #bb-bulk-bar doesn't exist yet; if we
  // ever take that earlier path the bar styles would never apply.
  if (window.Alpine && Alpine.store && Alpine.store('bulk')) {
    Alpine.store('bulk')._updateRowVisibility();
    Alpine.store('bulk')._syncAllRows();
  }
});
