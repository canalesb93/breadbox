// Design sandbox gallery Alpine component for /design.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// Owns the inline section filter, per-group open state, the page-scoped
// keyboard wiring, and the scroll-spy that highlights the topmost visible
// section in the outline sidebar. Sets shortcuts scope to 'design' on
// mount so the global `/` binding (which normally opens the command
// palette) gets shadowed by the design-page binding registered here —
// pressing `/` focuses the filter input. `cmd+k` is intercepted via a
// capture-phase window listener so commandPalette()'s @keydown.window
// handler in base.html never fires on this page; we focus the filter
// instead.

document.addEventListener('alpine:init', function () {
  Alpine.data('designGallery', function () {
    return {
      filter: '',
      // Slug of the section that's currently topmost on screen. Drives
      // the active-state styling on every sidebar link.
      activeSlug: '',
      // slug → group-slug lookup, seeded from the JSONScript tag emitted
      // by the templ. Used to auto-expand the containing collapse when a
      // section becomes active.
      _groupMap: {},
      _scrollHandler: null,
      _scrollTicking: false,

      // Per-group accordion state. Keys match the slugs in
      // DesignSectionGroups() (design_types.go). Initialized to all open
      // so the catalog is fully visible on first load.
      open: {
        foundations: true,
        layout: true,
        navigation: true,
        forms: true,
        data: true,
        feedback: true,
        patterns: true,
      },

      init: function () {
        var reg = window.Alpine && Alpine.store('shortcuts');
        if (reg) {
          reg.setScope('design');
          var self = this;
          reg.register({
            id: 'design.focus-filter',
            keys: '/',
            description: 'Focus section filter',
            group: 'Actions',
            scope: 'design',
            action: function () { self.focusFilter(); },
          });
        }

        // Seed the section→group map from the JSONScript emitted in
        // the templ. Missing tag = no scroll-spy, but the rest of the
        // page still works.
        try {
          var el = document.getElementById('design-section-groups');
          if (el) this._groupMap = JSON.parse(el.textContent) || {};
        } catch (_) { this._groupMap = {}; }

        this.setupScrollSpy();
      },

      destroy: function () {
        var reg = window.Alpine && Alpine.store('shortcuts');
        if (reg) {
          reg.unregister('design.focus-filter');
          reg.setScope('global');
        }
        if (this._scrollHandler) {
          window.removeEventListener('scroll', this._scrollHandler);
          window.removeEventListener('resize', this._scrollHandler);
        }
      },

      // True iff the section's title matches the active filter (case-insensitive
      // substring). Always true when filter is empty.
      match: function (t) {
        if (!this.filter) return true;
        return (t || '').toLowerCase().indexOf(this.filter.toLowerCase()) !== -1;
      },

      // True if ANY of the group's section titles match. Drives the
      // group's x-show so empty groups disappear while filtering. titles
      // is a pipe-joined string baked at render time by the templ.
      groupMatches: function (titles) {
        if (!this.filter) return true;
        return (titles || '').toLowerCase().indexOf(this.filter.toLowerCase()) !== -1;
      },

      // Active filter forces every group open so matched sections aren't
      // hidden behind a collapsed accordion.
      isGroupOpen: function (slug) {
        return !!this.filter || !!this.open[slug];
      },

      toggleGroup: function (slug) {
        this.open[slug] = !this.open[slug];
      },

      // Derived from the per-group map so the collapse-all/expand-all
      // label flips in response to individual group clicks too.
      get allOpen() {
        var keys = Object.keys(this.open);
        for (var i = 0; i < keys.length; i++) {
          if (!this.open[keys[i]]) return false;
        }
        return true;
      },

      toggleAll: function () {
        var next = !this.allOpen;
        var keys = Object.keys(this.open);
        for (var i = 0; i < keys.length; i++) this.open[keys[i]] = next;
      },

      focusFilter: function () {
        var el = this.$refs.filter;
        if (!el) return;
        el.focus();
        el.select();
      },

      // Capture-phase handler that intercepts cmd+k / ctrl+k before
      // commandPalette()'s @keydown.window listener in base.html can
      // open the palette. stopImmediatePropagation prevents the bubble-
      // phase listener on window from firing.
      onCmdK: function (e) {
        if (!((e.metaKey || e.ctrlKey) && e.key === 'k')) return;
        e.preventDefault();
        e.stopImmediatePropagation();
        this.focusFilter();
      },

      // Sets up a rAF-throttled scroll listener that finds the topmost
      // visible section and reflects it in `activeSlug`. Uses the
      // navbar offset (5.5rem = 88px) — matches the `scroll-mt-[5.5rem]`
      // applied to each section so a section becomes "active" the
      // moment its sticky-anchor target crosses the navbar line.
      setupScrollSpy: function () {
        var self = this;
        function sections() {
          return Array.from(document.querySelectorAll('[data-design-section]'));
        }
        if (!sections().length) return;

        var navbarOffset = 88 + 1;

        function pickActive() {
          var list = sections();
          var active = '';
          for (var i = 0; i < list.length; i++) {
            var s = list[i];
            // x-show hides via display:none → offsetParent is null. Skip
            // filtered-out sections so the active state ignores them.
            if (s.offsetParent === null) continue;
            var top = s.getBoundingClientRect().top;
            if (top <= navbarOffset) {
              active = s.id;
            } else if (active) {
              // First section below the cutoff after we already locked
              // one in — stop. activeSlug stays on the highest one above.
              break;
            } else {
              // Nothing has crossed the cutoff yet (page is at the very
              // top). Highlight the first visible section anyway so the
              // outline reflects what's on screen.
              active = s.id;
              break;
            }
          }
          if (active && active !== self.activeSlug) {
            self.activeSlug = active;
            self.ensureGroupOpen(active);
            self.scrollSidebarToActive(active);
          }
        }

        function onScroll() {
          if (self._scrollTicking) return;
          self._scrollTicking = true;
          window.requestAnimationFrame(function () {
            self._scrollTicking = false;
            pickActive();
          });
        }

        this._scrollHandler = onScroll;
        window.addEventListener('scroll', onScroll, { passive: true });
        window.addEventListener('resize', onScroll, { passive: true });

        // Initial pick on next frame so x-show has applied display:none
        // to filtered sections before we measure offsets.
        window.requestAnimationFrame(pickActive);
      },

      // Force-open the collapse that contains the active section so the
      // highlighted link is actually visible in the sidebar. Leaves
      // other groups alone — collapsing a different group above the
      // active one is the user's choice.
      ensureGroupOpen: function (slug) {
        var group = this._groupMap[slug];
        if (group && !this.open[group]) this.open[group] = true;
      },

      // Scrolls the sidebar so the active link stays in view. Uses
      // `block: 'nearest'` so this is a no-op when the link is already
      // visible, and only nudges the scroll position when needed.
      scrollSidebarToActive: function (slug) {
        var link = document.querySelector('aside a[href="#' + slug + '"]');
        if (link && link.scrollIntoView) {
          link.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
        }
      },
    };
  });
});
