// Design sandbox gallery Alpine component for /design.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// Owns the inline section filter, per-group open state, and the page-scoped
// keyboard wiring. Sets shortcuts scope to 'design' on mount so the global
// `/` binding (which normally opens the command palette) gets shadowed by
// the design-page binding registered here — pressing `/` focuses the
// filter input. `cmd+k` is intercepted via a capture-phase window
// listener so commandPalette()'s @keydown.window handler in base.html
// never fires on this page; we focus the filter instead.

document.addEventListener('alpine:init', function () {
  Alpine.data('designGallery', function () {
    return {
      filter: '',

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
        if (!reg) return;
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
      },

      destroy: function () {
        var reg = window.Alpine && Alpine.store('shortcuts');
        if (!reg) return;
        reg.unregister('design.focus-filter');
        reg.setScope('global');
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
    };
  });
});
