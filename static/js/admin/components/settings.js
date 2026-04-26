// Settings page scripts for /settings.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// This page does not use a named Alpine.data() factory — the root <div>
// uses an empty x-data plus x-init/x-destroy to set the keyboard-shortcut
// scope. One piece of behavior lives here:
//
//   1. If the URL contains a hash (#sync, #retention, #security, #help),
//      scroll the matching collapsible card into view after a short delay
//      so the Alpine x-show transition has time to expand it.
//
// The <script src> loads synchronously at the top of the templ component
// (so any future alpine:init listeners register before Alpine fires the
// event). That means this script runs BEFORE the rest of the body is
// parsed, so the hash-scroll function has to wait for DOMContentLoaded to
// query the target element.
function bbSettingsScrollToHash() {
  if (!location.hash) return;
  var el = document.querySelector(location.hash);
  if (el) {
    setTimeout(function () {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }, 100);
  }
}
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bbSettingsScrollToHash);
} else {
  bbSettingsScrollToHash();
}
