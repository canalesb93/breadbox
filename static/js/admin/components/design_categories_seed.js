// Seeds window.__bbCategories from whichever design-sandbox JSON script tag
// is present in the DOM. Used by SectionCategoryPicker and SectionTransactionRows
// in design_sections.templ so the category-picker Alpine factory can resolve
// category UUIDs to labels at first paint.
(function () {
    var ids = ["design-cat-picker-cats", "design-tx-rows-cats"];
    for (var i = 0; i < ids.length; i++) {
        var el = document.getElementById(ids[i]);
        if (el) {
            try { window.__bbCategories = JSON.parse(el.textContent); } catch (_) { window.__bbCategories = []; }
            break;
        }
    }
})();
