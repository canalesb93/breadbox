// OAuth-client-created Alpine component for /oauth-clients/{id}/created.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// Initial state — the freshly minted plaintext client_secret — is passed
// in via a data-secret attribute on the x-data root because it's a
// single scalar (no JSONScript needed). The Client ID and MCP server URL
// are read directly off their respective <input> fields by element id,
// matching the previous inline pattern; the secret is read from the
// dataset so the JS doesn't depend on a particular DOM ordering.
document.addEventListener('alpine:init', function () {
  Alpine.data('oauthClientCreated', function () {
    return {
      copiedId: false,
      copiedSecret: false,
      copiedUrl: false,

      copyClientId: function () {
        var el = document.getElementById('oauth-client-id');
        if (!el) return;
        var self = this;
        navigator.clipboard.writeText(el.value).then(function () {
          self.copiedId = true;
          setTimeout(function () { self.copiedId = false; }, 2000);
        });
      },

      copyClientSecret: function () {
        var secret = this.$el.dataset.secret || '';
        var self = this;
        navigator.clipboard.writeText(secret).then(function () {
          self.copiedSecret = true;
          setTimeout(function () { self.copiedSecret = false; }, 2000);
        });
      },

      copyMCPUrl: function () {
        var el = document.getElementById('oauth-mcp-url');
        if (!el) return;
        var self = this;
        navigator.clipboard.writeText(el.value).then(function () {
          self.copiedUrl = true;
          setTimeout(function () { self.copiedUrl = false; }, 2000);
        });
      },
    };
  });
});
