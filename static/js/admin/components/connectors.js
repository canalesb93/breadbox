// Connectors settings page factory (/settings/connectors). Drives the add/edit
// modal and the import-by-JSON modal for the global custom-MCP connector
// library. Initial connector list is hydrated from
//   <script id="connectors-data" type="application/json">[...]</script>
// via @templ.JSONScript. Header VALUES are never present in that payload — on
// edit the value inputs render blank and an empty submit keeps the stored value.
//
// Convention: docs/design-system.md → "Alpine page components".

document.addEventListener('alpine:init', function () {
  Alpine.data('connectors', function () {
    return {
      // Directory data keyed by short_id, parsed from the JSONScript tag.
      byId: {},

      // Add/edit modal state.
      formOpen: false,
      importOpen: false,
      mode: 'add', // 'add' | 'edit'
      shortId: '',
      name: '',
      url: '',
      transport: 'http',
      note: '',
      headers: [], // [{ name, value, stored }]
      confirmingDelete: false,

      init: function () {
        var list = [];
        var tag = document.getElementById('connectors-data');
        if (tag) {
          try { list = JSON.parse(tag.textContent || '[]'); } catch (e) { list = []; }
        }
        var map = {};
        (Array.isArray(list) ? list : []).forEach(function (c) { map[c.short_id] = c; });
        this.byId = map;
      },

      // ---- add / edit ----------------------------------------------------
      resetForm: function () {
        this.shortId = '';
        this.name = '';
        this.url = '';
        this.transport = 'http';
        this.note = '';
        this.confirmingDelete = false;
      },

      openAdd: function () {
        this.resetForm();
        this.mode = 'add';
        // Start with a single blank header row as an affordance.
        this.headers = [{ name: '', value: '', stored: false }];
        this.formOpen = true;
      },

      openEdit: function (shortId) {
        var c = this.byId[shortId];
        if (!c) return;
        this.resetForm();
        this.mode = 'edit';
        this.shortId = shortId;
        this.name = c.name || '';
        this.url = c.url || '';
        this.transport = c.transport || 'http';
        this.note = c.note || '';
        // Header values are write-only — pre-fill names, leave values blank.
        this.headers = (Array.isArray(c.headers) ? c.headers : []).map(function (n) {
          return { name: n, value: '', stored: true };
        });
        this.formOpen = true;
      },

      closeForm: function () {
        this.formOpen = false;
        this.confirmingDelete = false;
      },

      addHeader: function () {
        this.headers.push({ name: '', value: '', stored: false });
      },
      removeHeader: function (i) {
        this.headers.splice(i, 1);
      },

      formAction: function () {
        return this.mode === 'edit'
          ? '/-/connectors/' + encodeURIComponent(this.shortId) + '/update'
          : '/-/connectors';
      },
      deleteAction: function () {
        return '/-/connectors/' + encodeURIComponent(this.shortId) + '/delete';
      },
      submitDelete: function () {
        if (this.$refs.deleteForm) this.$refs.deleteForm.submit();
      },

      // ---- import --------------------------------------------------------
      openImport: function () {
        this.importOpen = true;
      },
    };
  });
});
