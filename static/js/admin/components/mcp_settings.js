// MCP Settings tab on /agents — three editable settings cards
// (Server Instructions, Review Guidelines, Report Format) plus
// Tools Enabled (form only) and Connection (copy snippet). Initial
// content + defaults flow in via @templ.JSONScript("mcp-settings-data").
//
// Convention reference: docs/design-system.md → "Alpine page components".
document.addEventListener('alpine:init', function () {
  function readData() {
    var el = document.getElementById('mcp-settings-data');
    if (!el) return {};
    try {
      return JSON.parse(el.textContent) || {};
    } catch (e) {
      console.error('mcpSettings: failed to parse #mcp-settings-data', e);
      return {};
    }
  }

  // mcpSettingsCard backs each of the three editable cards (instructions,
  // review-guidelines, report-format). The card's slug determines which
  // data-* keys it reads from the shared payload.
  Alpine.data('mcpSettingsCard', function () {
    return {
      expanded: false,
      value: '',
      defaultValue: '',
      slug: '',
      init: function () {
        this.slug = this.$el.dataset.slug || '';
        this.expanded = location.hash === '#' + this.slug;
        var data = readData();
        var keys = {
          instructions: ['instructions', 'defaultInstructions'],
          'review-guidelines': ['reviewGuidelines', 'defaultReviewGuidelines'],
          'report-format': ['reportFormat', 'defaultReportFormat'],
        }[this.slug];
        if (keys) {
          this.value = data[keys[0]] || '';
          this.defaultValue = data[keys[1]] || '';
        }
      },
      resetToDefaults: function () {
        var self = this;
        var msg = {
          instructions: 'Your current instructions will be replaced with the default server instructions.',
          'review-guidelines': 'Your review guidelines will be replaced with the defaults.',
          'report-format': 'Your report format will be replaced with the defaults.',
        }[this.slug] || 'This will be replaced with the defaults.';
        bbConfirm({ title: 'Reset to defaults?', message: msg, confirmLabel: 'Reset', variant: 'warning' }).then(function (ok) {
          if (ok) self.value = self.defaultValue;
        });
      },
    };
  });

  // mcpSettingsConnection backs the Connection card — only needs an
  // expand toggle and the inline copy state.
  Alpine.data('mcpSettingsConnection', function () {
    return {
      expanded: false,
      copiedConfig: false,
      init: function () {
        this.expanded = location.hash === '#connection';
      },
      copyStdioConfig: function () {
        var self = this;
        var cfg = JSON.stringify(
          { mcpServers: { breadbox: { command: 'docker', args: ['exec', '-i', 'breadbox-app-1', '/app/breadbox', 'mcp-stdio'] } } },
          null,
          2
        );
        navigator.clipboard.writeText(cfg).then(function () {
          self.copiedConfig = true;
          setTimeout(function () {
            self.copiedConfig = false;
          }, 2000);
        });
      },
    };
  });

  // mcpSettingsTools backs the Tools Enabled card — expand toggle only.
  Alpine.data('mcpSettingsTools', function () {
    return {
      expanded: false,
      init: function () {
        this.expanded = location.hash === '#tools';
      },
    };
  });

  // Deep-link: scroll to the targeted section once the page has rendered.
  if (location.hash) {
    setTimeout(function () {
      var el = document.querySelector(location.hash);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }, 100);
  }
});
