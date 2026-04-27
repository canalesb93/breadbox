// Prompt Library tab on /agents — list of preset prompt cards. Each
// card has its own copy button that fetches the composed prompt from
// /agent-wizard/<slug>/copy and writes it to the clipboard.
//
// Convention reference: docs/design-system.md → "Alpine page components".
document.addEventListener('alpine:init', function () {
  Alpine.data('agentWizardCard', function () {
    return {
      copied: false,
      copyPrompt: function (slug) {
        var self = this;
        fetch('/agent-wizard/' + slug + '/copy')
          .then(function (r) {
            return r.text();
          })
          .then(function (t) {
            return navigator.clipboard.writeText(t);
          })
          .then(function () {
            self.copied = true;
            setTimeout(function () {
              self.copied = false;
            }, 2000);
          });
      },
    };
  });
});
