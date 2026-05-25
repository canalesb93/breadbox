// /agent-prompts prompt-library cards. Each card has its own copy button
// that fetches the composed prompt from
// /agent-prompts/builder/<slug>/copy and writes it to the clipboard.
//
// Convention reference: docs/design-system.md → "Alpine page components".
document.addEventListener('alpine:init', function () {
  Alpine.data('agentWizardCard', function () {
    return {
      copied: false,
      copyPrompt: function (slug) {
        var self = this;
        function toast(message, type) {
          window.dispatchEvent(new CustomEvent('bb-toast', {
            detail: { message: message, type: type || 'error' },
          }));
        }
        fetch('/agent-prompts/builder/' + slug + '/copy')
          .then(function (r) {
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.text();
          })
          .then(function (t) {
            return navigator.clipboard.writeText(t);
          })
          .then(function () {
            self.copied = true;
            toast('Prompt copied to clipboard.', 'success');
            setTimeout(function () { self.copied = false; }, 2000);
          })
          .catch(function () {
            toast('Failed to copy prompt. Please try again.', 'error');
          });
      },
    };
  });
});
