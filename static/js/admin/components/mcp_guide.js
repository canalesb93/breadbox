// Getting Started tab on /agents — connection-method picker + per-client
// install snippets. Reads the MCP server URL from data-mcp-url on the
// x-data root (set by the templ component).
//
// Convention reference: docs/design-system.md → "Alpine page components".
document.addEventListener('alpine:init', function () {
  Alpine.data('mcpGuide', function () {
    function stdConfig(url) {
      return JSON.stringify(
        { mcpServers: { breadbox: { type: 'streamable-http', url: url, headers: { 'X-API-Key': 'YOUR_API_KEY' } } } },
        null,
        2
      );
    }

    return {
      tab: 'claude-desktop',
      flavor: 'guide',
      mcpURL: '',
      tabs: [
        { id: 'claude-desktop', label: 'Claude Desktop', icon: 'monitor', hasGuide: true },
        { id: 'claude-code', label: 'Claude Code', icon: 'terminal', hasGuide: false },
        { id: 'chatgpt', label: 'ChatGPT', icon: 'message-circle', hasGuide: true },
        { id: 'other', label: 'Other', icon: 'puzzle', hasGuide: true },
      ],

      get activeTab() {
        var self = this;
        return this.tabs.find(function (t) {
          return t.id === self.tab;
        });
      },
      get showFlavorToggle() {
        var t = this.activeTab;
        return t && t.hasGuide;
      },

      init: function () {
        this.mcpURL = this.$el.dataset.mcpUrl || '';
        // Render Lucide icons in all tab panels after Alpine hydrates.
        var self = this;
        setTimeout(function () {
          self.tabs.forEach(function (t) {
            var panel = self.$refs['panel-' + t.id];
            if (panel && typeof lucide !== 'undefined') lucide.createIcons({ nodes: [panel] });
          });
        }, 50);
      },

      switchTab: function (id) {
        this.tab = id;
        var t = this.tabs.find(function (tt) {
          return tt.id === id;
        });
        if (t && !t.hasGuide) this.flavor = 'config';
        else if (this.flavor === 'config' && t && t.hasGuide) this.flavor = 'guide';
        var self = this;
        setTimeout(function () {
          var panel = self.$refs['panel-' + id];
          if (panel && typeof lucide !== 'undefined') lucide.createIcons({ nodes: [panel] });
        }, 10);
      },

      configSnippet: function (service) {
        switch (service) {
          case 'claude-desktop':
          case 'claude-code':
            return stdConfig(this.mcpURL);
          case 'claude-code-cli':
            return 'claude mcp add breadbox --transport http \\\n  --url ' + this.mcpURL + ' \\\n  --header "X-API-Key: YOUR_API_KEY"';
          case 'other':
            return JSON.stringify(
              { breadbox: { type: 'streamable-http', url: this.mcpURL, headers: { 'X-API-Key': 'YOUR_API_KEY' } } },
              null,
              2
            );
          default:
            return this.mcpURL;
        }
      },

      copyAndFlash: function (text, ctx) {
        navigator.clipboard.writeText(text).then(function () {
          ctx.copied = true;
          setTimeout(function () {
            ctx.copied = false;
          }, 2000);
        });
      },
    };
  });
});
