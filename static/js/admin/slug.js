// Shared "auto-derive slug from display name" helper for admin forms that
// have both a Name/Display Name field and a Slug field side-by-side
// (agent form, tag create form).
//
// Exposes two globals:
//   - window.bbSlugify(text)
//     Returns a URL-safe slug (lowercase [a-z0-9-], max 64 chars, no
//     leading/trailing dash). Suitable for both the agent slug pattern
//     and the tag slug pattern (the tag pattern also allows colons, but
//     those aren't reachable from a typed name).
//
//   - window.bbWireSlugInput(nameId, slugId)
//     Wires a name input to populate a slug input as the user types — but
//     only while the user hasn't touched the slug field themselves. Edit
//     forms whose slug starts pre-populated are treated as "already
//     touched" so the user-chosen slug is never overwritten.
//
// We detect "user typed in the slug" via Event.isTrusted, which is true
// for real keystrokes and false for our own dispatchEvent calls. That
// lets us synthesise an `input` event after writing the new slug — so
// any Alpine `x-model` binding picks up the new value — without flipping
// the "touched" flag ourselves.
//
// Accented characters survive the NFKD pass as their base letter plus a
// combining mark; the [^a-z0-9]+ collapse drops the mark together with
// any spaces, so "Café Bar" -> "cafe-bar" without an explicit diacritic
// strip step.

(function () {
  function slugify(name) {
    return String(name == null ? '' : name)
      .normalize('NFKD')
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 64);
  }

  function wire(nameId, slugId) {
    var nameEl = document.getElementById(nameId);
    var slugEl = document.getElementById(slugId);
    if (!nameEl || !slugEl) return;
    var touched = !!slugEl.value;
    slugEl.addEventListener('input', function (e) {
      if (e.isTrusted) touched = true;
    });
    nameEl.addEventListener('input', function () {
      if (touched) return;
      var next = slugify(nameEl.value);
      if (next === slugEl.value) return;
      slugEl.value = next;
      slugEl.dispatchEvent(new Event('input', { bubbles: true }));
    });
  }

  window.bbSlugify = slugify;
  window.bbWireSlugInput = wire;
})();
