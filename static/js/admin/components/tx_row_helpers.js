// Shared row-update helpers for pages that render `tx_row.templ` with the
// inline category picker (currently /transactions, /account/{id}, /rules/{id}).
//
// The inline picker's `x-init` watcher fires `quickSetCategory(txId, id)`
// when the user picks a new category. The button label updates reactively
// via Alpine (selectedId → displayLabel), but the rest of the row —
// avatar icon/color, the xl-hidden compact category label — is server-
// rendered and stays stale until JS rewrites it. `updateRowCategory()` is
// the canonical "rewrite the row's category-derived UI" function; pages
// call it after a successful POST so the change is visible without reload.
//
// Loaded as a separate <script src> from each parent page's templ so all
// three quickSetCategory implementations can share one definition. Exposes
// helpers on `window` so the per-page factories can call them.

/* ── In-place category update on a row ── */
function updateRowCategory(txId, slug) {
  if (!txId) return;
  var row = document.querySelector('.bb-tx-row[data-tx-id="' + CSS.escape(txId) + '"]');
  if (!row) return;

  // Look up category metadata (icon, color, display_name, id) by slug.
  // window.__bbCategories is the parent → children tree; flatten on the fly.
  var meta = null, parentMeta = null;
  if (slug) {
    var cats = window.__bbCategories || [];
    for (var i = 0; i < cats.length && !meta; i++) {
      if (cats[i].slug === slug) { meta = cats[i]; break; }
      if (cats[i].children) {
        for (var j = 0; j < cats[i].children.length; j++) {
          if (cats[i].children[j].slug === slug) {
            meta = cats[i].children[j];
            parentMeta = cats[i];
            break;
          }
        }
      }
    }
  }

  // 1) Update the avatar (or convert a letter-avatar to an icon-avatar).
  // Build via DOM APIs (textContent + a strict icon-name whitelist) instead
  // of innerHTML — display_name is admin-controlled and could otherwise
  // smuggle stored XSS through the category metadata.
  var avatarWrap = row.querySelector('.bb-tx-avatar') && row.querySelector('.bb-tx-avatar').parentElement;
  if (avatarWrap) {
    var color = (meta && (meta.color || (parentMeta && parentMeta.color))) || 'oklch(0.65 0 0)';
    var icon = meta && (meta.icon || (parentMeta && parentMeta.icon));
    var avatar = document.createElement('div');
    if (_safeLucideName(icon)) {
      avatar.className = 'bb-tx-avatar';
      avatar.style.setProperty('--avatar-color', color);
      var iconEl = document.createElement('i');
      iconEl.setAttribute('data-lucide', icon);
      iconEl.className = 'w-4 h-4';
      avatar.appendChild(iconEl);
      avatarWrap.replaceChildren(avatar);
      if (typeof lucide !== 'undefined') lucide.createIcons({ nodes: [avatarWrap] });
    } else {
      var name = (row.querySelector('a[href*="/transactions/"]') && row.querySelector('a[href*="/transactions/"]').textContent.trim()) || '?';
      avatar.className = 'bb-tx-avatar bb-tx-avatar--letter';
      var letter = document.createElement('span');
      letter.textContent = name.charAt(0).toUpperCase();
      avatar.appendChild(letter);
      avatarWrap.replaceChildren(avatar);
    }
  }

  // 2) Update the inline cat picker (Alpine-reactive: writing selectedId
  // re-derives displayLabel/displayColor/displayIcon). The picker has an
  // x-init $watch on selectedId that fires quickSetCategory; that watcher
  // checks $store.bulk.processing and skips when bulk ops are in flight,
  // so we don't double-call the API or double-toast on top of the bulk
  // action. For the single-row path (this function is also called from
  // quickSetCategory's success branch) the picker already holds the new
  // id, so this assignment is a no-op and the watcher won't re-fire.
  var pickerEl = row.querySelector('.bb-cat-picker');
  if (pickerEl && window.Alpine && Alpine.$data) {
    var data = Alpine.$data(pickerEl);
    if (data) data.selectedId = (meta && meta.id) || '';
  }

  // 3) Update the compact (xl-hidden) category label. The row template
  // uses the `xl` breakpoint to swap between the inline picker (xl+) and
  // a compact "Category" / "Uncategorized" label below xl. Use textContent
  // for display_name (admin-controlled) so HTML in a category name can't
  // execute via this update path.
  var mobileLabel = row.querySelector('span.xl\\:hidden.inline-flex.items-center.gap-1, span.xl\\:hidden.text-base-content\\/25');
  if (mobileLabel) {
    var newLabel = document.createElement('span');
    if (meta) {
      newLabel.className = 'xl:hidden inline-flex items-center gap-1 shrink min-w-0';
      var labelColor = (meta.color || (parentMeta && parentMeta.color)) || '';
      if (labelColor) {
        var dot = document.createElement('span');
        dot.className = 'w-1.5 h-1.5 rounded-full shrink-0';
        dot.style.backgroundColor = labelColor;
        newLabel.appendChild(dot);
      }
      var labelName = document.createElement('span');
      labelName.className = 'text-base-content/50 truncate';
      labelName.textContent = meta.display_name || meta.slug;
      newLabel.appendChild(labelName);
    } else {
      newLabel.className = 'xl:hidden text-base-content/25 truncate shrink-0';
      newLabel.textContent = 'Uncategorized';
    }
    mobileLabel.replaceWith(newLabel);
  }
}

// Lucide icon names are slug-shaped (e.g. "trending-up"). Reject anything
// else so a malicious icon value can't break out of the data-lucide
// attribute and inject scripts when set via innerHTML elsewhere.
function _safeLucideName(name) {
  if (!name || typeof name !== 'string') return null;
  return /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name) ? name : null;
}

// Look up a category slug by id from the window.__bbCategories tree
// (parents + children). Returns '' if not found.
function _slugForCategoryId(id) {
  if (!id) return '';
  var cats = window.__bbCategories || [];
  for (var i = 0; i < cats.length; i++) {
    if (cats[i].id === id) return cats[i].slug;
    var children = cats[i].children || [];
    for (var j = 0; j < children.length; j++) {
      if (children[j].id === id) return children[j].slug;
    }
  }
  return '';
}

window.updateRowCategory = updateRowCategory;
window._slugForCategoryId = _slugForCategoryId;
