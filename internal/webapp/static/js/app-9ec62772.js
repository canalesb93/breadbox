// Breadbox v3 webapp — tiny progressive-enhancement layer.
// Hand-written, no bundler needed yet. Real islands (⌘K palette, drag-drop) arrive
// in Phase 4 via esbuild-via-Go. Everything here degrades gracefully without JS.
(function () {
  "use strict";

  // --- Theme toggle: persist to a cookie so the server can render the right class
  //     on first paint (no flash). The <html> class is set server-side already. ---
  function applyTheme(theme) {
    const root = document.documentElement;
    if (theme === "dark") root.classList.add("dark");
    else if (theme === "light") root.classList.remove("dark");
    else {
      // system
      const dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      root.classList.toggle("dark", dark);
    }
    document.cookie =
      "bb_theme=" + encodeURIComponent(theme) + ";path=/;max-age=31536000;samesite=lax";
  }

  document.addEventListener("click", function (e) {
    const btn = e.target.closest("[data-theme-set]");
    if (btn) {
      e.preventDefault();
      applyTheme(btn.getAttribute("data-theme-set"));
    }
  });

  // --- Popover/menu placement fallback (CSS Anchor Positioning isn't in Safari/FF yet).
  //     For [popover] elements with a data-anchor target, position below-left of the anchor. ---
  function placePopover(pop) {
    const anchorId = pop.getAttribute("data-anchor");
    if (!anchorId) return;
    const anchor = document.getElementById(anchorId);
    if (!anchor) return;
    const r = anchor.getBoundingClientRect();
    pop.style.position = "fixed";
    pop.style.top = r.bottom + 6 + "px";
    pop.style.left = r.left + "px";
    pop.style.margin = "0";
  }
  document.addEventListener("toggle", function (e) {
    const pop = e.target;
    if (pop.matches && pop.matches("[popover][data-anchor]") && e.newState === "open") {
      placePopover(pop);
    }
  }, true);
})();
