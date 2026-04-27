// Prompt builder Alpine component for /agent-prompts/builder/{type}.
//
// Convention reference: docs/design-system.md → "Alpine page components".
// Initial blocks payload is rendered server-side as
// <script id="prompt-builder-data" type="application/json">[...]</script>
// via @templ.JSONScript and parsed once in init().
document.addEventListener('alpine:init', function () {
  Alpine.data('promptBuilder', function () {
    var customCounter = 0;

    return {
      blocks: [],
      copied: false,
      copiedBlockId: null,
      showPreview: false,
      draggingId: null,
      focusedBlock: null,
      collapsedBlocks: {},
      blockHeights: {},

      // Blocks that are mutually exclusive — enabling one disables the others in the same group.
      exclusiveGroups: {
        'review-depth': ['review-depth-efficient', 'review-depth-thorough']
      },

      init: function () {
        var initialBlocks = [];
        var dataEl = document.getElementById('prompt-builder-data');
        if (dataEl) {
          try {
            initialBlocks = JSON.parse(dataEl.textContent) || [];
          } catch (e) {
            console.error('promptBuilder: failed to parse #prompt-builder-data', e);
            initialBlocks = [];
          }
        }
        this.blocks = initialBlocks.map(function (b) {
          b.isCustom = false;
          return b;
        });
      },

      getBlockHeight: function (id) {
        return this.blockHeights[id] || 2000;
      },

      updateBlockHeight: function (id, el) {
        var wrapper = el.closest('.px-4');
        if (wrapper) {
          this.blockHeights[id] = wrapper.scrollHeight + 20;
        }
      },

      isCollapsed: function (id) {
        return !!this.collapsedBlocks[id];
      },

      toggleCollapse: function (id) {
        this.collapsedBlocks[id] = !this.collapsedBlocks[id];
        if (!this.collapsedBlocks[id]) {
          var self = this;
          this.$nextTick(function () {
            var el = self.$el.querySelector('[data-block-id="' + id + '"] textarea');
            if (el) {
              self.autoResize(el);
              self.updateBlockHeight(id, el);
            }
          });
        }
      },

      get activeBlocks() {
        return this.blocks.filter(function (b) { return b.enabled; });
      },

      // All non-core, non-custom blocks (for the pill toggles at top)
      get coreBlocks() {
        return this.blocks.filter(function (b) { return b.role === 'core'; });
      },

      get allToggleableBlocks() {
        return this.blocks.filter(function (b) { return b.role !== 'core' && !b.isCustom; });
      },

      get nonGroupToggleableBlocks() {
        var self = this;
        return this.allToggleableBlocks.filter(function (b) { return !self.isInExclusiveGroup(b.id); });
      },

      get exclusiveGroupEntries() {
        var self = this;
        var result = [];
        var seen = {};
        this.allToggleableBlocks.forEach(function (b) {
          var group = self.getExclusiveGroup(b.id);
          if (group) {
            var key = group.join(',');
            if (!seen[key]) {
              seen[key] = true;
              var members = group.map(function (gid) {
                return self.blocks.find(function (bl) { return bl.id === gid; });
              }).filter(Boolean);
              if (members.length > 0) result.push(members);
            }
          }
        });
        return result;
      },

      get preview() {
        return this.blocks
          .filter(function (b) { return b.enabled; })
          .map(function (b) { return b.content; })
          .join('\n\n');
      },

      getBlockIcon: function (id) {
        var icons = {
          'base-context': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H19a1 1 0 0 1 1 1v18a1 1 0 0 1-1 1H6.5a1 1 0 0 1 0-5H20"/></svg>',
          'strategy-initial-setup': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 3-1.9 5.8a2 2 0 0 1-1.287 1.288L3 12l5.8 1.9a2 2 0 0 1 1.288 1.288L12 21l1.9-5.8a2 2 0 0 1 1.287-1.288L21 12l-5.8-1.9a2 2 0 0 1-1.288-1.288Z"/></svg>',
          'strategy-bulk-review': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 3h5v5"/><path d="M8 3H3v5"/><path d="M12 22v-8.3a4 4 0 0 0-1.172-2.872L3 3"/><path d="m15 9 6-6"/></svg>',
          'strategy-quick-review': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/></svg>',
          'strategy-routine-review': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 2.1l4 4-4 4"/><path d="M3 12.2v-2.1a4 4 0 0 1 4-4h12.8M7 21.9l-4-4 4-4"/><path d="M21 11.8v2.1a4 4 0 0 1-4 4H4.2"/></svg>',
          'strategy-spending-report': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18"/><path d="m19 9-5 5-4-4-3 3"/></svg>',
          'strategy-anomaly-detection': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg>',
          'tool-reference': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>',
          'category-system': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"/><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"/></svg>',
          'rule-creation': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M7 12h10"/><path d="M10 18h4"/></svg>',
          'report-submission': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/></svg>',
          'account-linking': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m18 16 4-4-4-4"/><path d="m6 8-4 4 4 4"/><path d="m14.5 4-5 16"/></svg>',
          'teller-categories': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="3" x2="21" y1="22" y2="22"/><line x1="6" x2="6" y1="18" y2="11"/><line x1="10" x2="10" y1="18" y2="11"/><line x1="14" x2="14" y1="18" y2="11"/><line x1="18" x2="18" y1="18" y2="11"/><polygon points="12 2 20 7 4 7"/></svg>',
          'token-efficiency': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/></svg>',
          'scale-guidance': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21 16-4 4-4-4"/><path d="M17 20V4"/><path d="m3 8 4-4 4 4"/><path d="M7 4v16"/></svg>',
          'gmail-integration': '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="12" viewBox="0 0 256 193"><path fill="#4285F4" d="M58.182 192.05V93.14L27.507 65.077 0 49.504v125.091c0 9.658 7.825 17.455 17.455 17.455z"/><path fill="#34A853" d="M197.818 192.05h40.727c9.659 0 17.455-7.826 17.455-17.455V49.505l-31.156 17.837-27.026 25.798z"/><path fill="#EA4335" d="m58.182 93.14-4.174-38.647 4.174-36.989L128 69.868l69.818-52.364 4.669 34.992-4.669 40.644L128 145.504z"/><path fill="#FBBC04" d="M197.818 17.504V93.14L256 49.504V26.231c0-21.585-24.64-33.89-41.89-20.945z"/><path fill="#C5221F" d="M0 49.504l26.759 20.07L58.182 93.14V17.504L41.89 5.286C24.61-7.66 0 4.646 0 26.23z"/></svg>',
          'sync-management': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/></svg>',
          'transaction-comments': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7.9 20A9 9 0 1 0 4 16.1L2 22z"/></svg>',
          'merchant-analysis': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m2 7 4.41-4.41A2 2 0 0 1 7.83 2h8.34a2 2 0 0 1 1.42.59L22 7"/><path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8"/><path d="M15 22v-4a2 2 0 0 0-2-2h-2a2 2 0 0 0-2 2v4"/><path d="M2 7h20"/><path d="M22 7v3a2 2 0 0 1-2 2a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 16 12a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 12 12a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 8 12a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 4 12a2 2 0 0 1-2-2V7"/></svg>',
          'review-depth-efficient': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/></svg>',
          'review-depth-thorough': '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>'
        };
        return icons[id] || '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/></svg>';
      },

      autoResize: function (el) {
        el.style.height = '0';
        el.style.height = el.scrollHeight + 'px';
      },

      // --- Drag & drop ---
      handleDragStart: function (e, index, id) {
        this.draggingId = id;
        this.focusedBlock = null;
        // Blur any focused textarea
        if (document.activeElement && document.activeElement.tagName === 'TEXTAREA') {
          document.activeElement.blur();
        }
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', id);
        var el = e.currentTarget;
        if (el) {
          var rect = el.getBoundingClientRect();
          var offsetX = e.clientX - rect.left;
          var offsetY = e.clientY - rect.top;
          e.dataTransfer.setDragImage(el, offsetX, offsetY);
        }
      },

      handleDragOver: function (e, toIndex) {
        if (this.draggingId === null) return;

        var activeIds = this.activeBlocks.map(function (b) { return b.id; });
        var fromActiveIdx = activeIds.indexOf(this.draggingId);
        if (fromActiveIdx === -1 || fromActiveIdx === toIndex) return;

        var el = e.currentTarget;
        var rect = el.getBoundingClientRect();
        var midY = rect.top + rect.height / 2;
        var insertBefore = e.clientY < midY;

        var targetActiveIdx = insertBefore ? toIndex : toIndex + 1;
        if (targetActiveIdx === fromActiveIdx || targetActiveIdx === fromActiveIdx + 1) return;

        var enabled = [];
        for (var i = 0; i < this.blocks.length; i++) {
          if (this.blocks[i].enabled) enabled.push(i);
        }

        var fromBlockIdx = enabled[fromActiveIdx];
        var toBlockIdx = targetActiveIdx < enabled.length ? enabled[targetActiveIdx] : this.blocks.length;

        var item = this.blocks.splice(fromBlockIdx, 1)[0];
        if (fromBlockIdx < toBlockIdx) toBlockIdx--;
        this.blocks.splice(toBlockIdx, 0, item);
      },

      handleDragEnd: function () {
        this.draggingId = null;
        this.resizeAllTextareas();
      },

      handleDrop: function () {
        this.draggingId = null;
        this.resizeAllTextareas();
      },

      resizeAllTextareas: function () {
        var self = this;
        this.$nextTick(function () {
          setTimeout(function () {
            self.$el.querySelectorAll('textarea').forEach(function (ta) {
              self.autoResize(ta);
              var blockEl = ta.closest('[data-block-id]');
              if (blockEl) {
                self.updateBlockHeight(blockEl.dataset.blockId, ta);
              }
            });
          }, 50);
        });
      },

      getExclusiveGroup: function (id) {
        var groups = this.exclusiveGroups;
        for (var key in groups) {
          if (groups[key].indexOf(id) !== -1) return groups[key];
        }
        return null;
      },

      isInExclusiveGroup: function (id) {
        return this.getExclusiveGroup(id) !== null;
      },

      toggleBlock: function (id) {
        var block = this.blocks.find(function (b) { return b.id === id; });
        if (block && block.role !== 'core') {
          var group = this.getExclusiveGroup(id);
          if (group && !block.enabled) {
            // Enabling: disable siblings in the group
            var self = this;
            group.forEach(function (siblingId) {
              if (siblingId !== id) {
                var sib = self.blocks.find(function (b) { return b.id === siblingId; });
                if (sib) sib.enabled = false;
              }
            });
          }
          block.enabled = !block.enabled;
          if (block.enabled) {
            delete this.collapsedBlocks[id];
            this.resizeAllTextareas();
          }
        }
      },

      addCustomBlock: function () {
        customCounter++;
        var id = 'custom-' + customCounter;
        this.blocks.push({
          id: id,
          title: 'Custom Instructions',
          description: 'Your own instructions — edit freely',
          content: '',
          originalContent: '',
          role: 'optional',
          enabled: true,
          isCustom: true
        });
        var self = this;
        this.$nextTick(function () {
          var el = self.$el.querySelector('[data-block-id="' + id + '"] textarea');
          if (el) el.focus();
        });
      },

      removeCustomBlock: function (id) {
        this.blocks = this.blocks.filter(function (b) { return b.id !== id; });
        delete this.collapsedBlocks[id];
      },

      resetBlock: function (id) {
        var block = this.blocks.find(function (b) { return b.id === id; });
        if (block) {
          block.content = block.originalContent;
          // Reset stored height so it recalculates
          delete this.blockHeights[id];
          var self = this;
          this.$nextTick(function () {
            var el = self.$el.querySelector('[data-block-id="' + id + '"] textarea');
            if (el) {
              self.autoResize(el);
              // Double-nextTick to ensure DOM has updated with new content
              self.$nextTick(function () {
                self.autoResize(el);
                self.updateBlockHeight(id, el);
              });
            }
          });
        }
      },

      copyBlock: function (id) {
        var block = this.blocks.find(function (b) { return b.id === id; });
        if (!block) return;
        var self = this;
        navigator.clipboard.writeText(block.content).then(function () {
          self.copiedBlockId = id;
          setTimeout(function () { self.copiedBlockId = null; }, 2000);
        });
      },

      openPreview: function () {
        if (this.activeBlocks.length === 0) {
          this.showToast('Enable at least one block to preview', 'info');
          return;
        }
        this.showPreview = true;
        document.body.style.overflow = 'hidden';
      },

      closePreview: function () {
        this.showPreview = false;
        document.body.style.overflow = '';
      },

      renderMarkdown: function (text) {
        if (typeof marked === 'undefined') {
          return '<pre style="white-space:pre-wrap">' + text.replace(/&/g, '&amp;').replace(/</g, '&lt;') + '</pre>';
        }
        // Preprocess: convert "ALL CAPS HEADER:" lines to markdown ## headers
        var processed = text.replace(/^([A-Z][A-Z _&\/()\-—]+):[ ]*$/gm, function (match, header) {
          var clean = header.trim();
          var titled = clean.toLowerCase().replace(/\b\w/g, function (c) { return c.toUpperCase(); });
          return '\n## ' + titled + '\n';
        });
        // Ensure blank lines before list items (marked requires them)
        processed = processed.replace(/([^\n])\n(- )/g, '$1\n\n$2');
        // Ensure blank lines before numbered list items
        processed = processed.replace(/([^\n])\n(\d+\. )/g, '$1\n\n$2');
        return marked.parse(processed);
      },

      showToast: function (msg, type) {
        window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: msg, type: type || 'success' } }));
      },

      copyPrompt: function () {
        if (this.activeBlocks.length === 0) {
          this.showToast('Enable at least one block to copy', 'info');
          return;
        }
        var self = this;
        navigator.clipboard.writeText(this.preview).then(function () {
          self.copied = true;
          self.showToast('Prompt copied to clipboard');
          setTimeout(function () { self.copied = false; }, 2000);
        });
      },

      handleKeyboard: function (e) {
        // Don't trigger shortcuts when typing in a textarea or input
        if (e.target.tagName === 'TEXTAREA' || e.target.tagName === 'INPUT') return;
        // Don't trigger if modal is open (except Escape which is handled separately)
        if (e.key === 'c' && !e.metaKey && !e.ctrlKey) {
          e.preventDefault();
          this.copyPrompt();
        } else if (e.key === 'p' && !e.metaKey && !e.ctrlKey) {
          e.preventDefault();
          if (this.showPreview) {
            this.closePreview();
          } else {
            this.openPreview();
          }
        }
      }
    };
  });
});
