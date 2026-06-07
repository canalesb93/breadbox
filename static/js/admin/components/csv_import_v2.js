// csvImport — Alpine factory for the drop-anywhere CSV import flow.
//
// Mounted once via components.ImportModal in base.html. The $store.csvImport
// store handles the drop overlay and parks the dropped File on pendingFile;
// this component watches the store's `seq` counter and runs the flow:
//   analyze → (account) → preview/edit → apply → result
// All steps are server-backed (admin /-/csv/v2/* endpoints). CSRF is injected
// automatically by the global fetch wrapper in base.html.
document.addEventListener('alpine:init', function () {
  Alpine.data('csvImport', function () {
    return {
      step: 'analyzing', // analyzing | error | account | preview | result
      loading: false,
      errorMsg: '',
      filename: '',
      sessionId: '',

      // account step
      accounts: [],
      selectedAccountId: '',
      createNew: false,
      newName: '',

      // preview step
      summary: {},
      rows: [],
      statusFilter: '',
      page: 1,
      pageSize: 50,
      hasMore: false,
      editingId: null,
      edit: { date: '', amount: '', description: '' },

      // result step
      result: {},

      init: function () {
        var self = this;
        this.$watch('$store.csvImport.seq', function () {
          var s = Alpine.store('csvImport');
          if (s && s.active && s.pendingFile) {
            self.analyze(s.pendingFile);
          }
        });
      },

      resetState: function () {
        this.step = 'analyzing';
        this.loading = false;
        this.errorMsg = '';
        this.sessionId = '';
        this.accounts = [];
        this.selectedAccountId = '';
        this.createNew = false;
        this.newName = '';
        this.summary = {};
        this.rows = [];
        this.statusFilter = '';
        this.page = 1;
        this.hasMore = false;
        this.editingId = null;
        this.result = {};
      },

      closeFlow: function () {
        var s = Alpine.store('csvImport');
        if (s) s.close();
        this.resetState();
      },

      fail: function (msg) {
        this.step = 'error';
        this.errorMsg = msg || 'Something went wrong';
        this.loading = false;
      },

      analyze: function (file) {
        var self = this;
        this.resetState();
        this.filename = file.name || 'CSV import';
        this.step = 'analyzing';
        var fd = new FormData();
        fd.append('file', file);
        fd.append('filename', file.name || '');
        fetch('/-/csv/v2/sessions', { method: 'POST', body: fd })
          .then(function (r) { return r.json().then(function (b) { return { ok: r.ok, body: b }; }); })
          .then(function (res) {
            if (!res.ok) { self.fail(self.errText(res.body)); return; }
            var a = res.body;
            self.sessionId = a.session.short_id;
            self.accounts = (a.accounts && a.accounts.matches) || [];
            self.summary = a.summary || {};
            if (a.session.status === 'previewed' && a.session.resolved_account_id) {
              self.page = 1;
              self.loadRows();
              self.step = 'preview';
            } else {
              var pre = a.accounts && a.accounts.preselect;
              if (pre) { self.selectedAccountId = pre; }
              else if (self.accounts.length === 0) { self.createNew = true; }
              self.step = 'account';
            }
          })
          .catch(function () { self.fail('Upload failed'); });
      },

      resolve: function () {
        var self = this;
        this.loading = true;
        var payload = this.createNew
          ? { create_new: true, new_name: this.newName || 'CSV Import' }
          : { account_id: this.selectedAccountId };
        fetch('/-/csv/v2/sessions/' + this.sessionId + '/resolve', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
          .then(function (r) { return r.json().then(function (b) { return { ok: r.ok, body: b }; }); })
          .then(function (res) {
            self.loading = false;
            if (!res.ok) { self.fail(self.errText(res.body)); return; }
            self.page = 1;
            self.step = 'preview';
            self.loadRows();
          })
          .catch(function () { self.loading = false; self.fail('Failed to set account'); });
      },

      loadRows: function () {
        var self = this;
        var qs = '?status=' + encodeURIComponent(this.statusFilter) +
          '&page=' + this.page + '&page_size=' + this.pageSize;
        fetch('/-/csv/v2/sessions/' + this.sessionId + '/rows' + qs)
          .then(function (r) { return r.json(); })
          .then(function (b) {
            self.rows = b.rows || [];
            self.summary = b.summary || {};
            self.hasMore = (b.rows || []).length === self.pageSize;
          })
          .catch(function () { /* keep current view */ });
      },

      setFilter: function (s) {
        this.statusFilter = s;
        this.page = 1;
        this.loadRows();
      },
      prevPage: function () { if (this.page > 1) { this.page--; this.loadRows(); } },
      nextPage: function () { if (this.hasMore) { this.page++; this.loadRows(); } },

      toggleRow: function (r) {
        var self = this;
        this.patchRow(r.id, {
          date: r.date, amount: r.amount, description: r.description,
          merchant: r.merchant, include: !r.include,
        }, function () { self.loadRows(); });
      },

      startEdit: function (r) {
        this.editingId = r.id;
        this.edit = { date: r.date, amount: r.amount, description: r.description };
      },

      saveEdit: function (r) {
        var self = this;
        this.patchRow(r.id, {
          date: this.edit.date, amount: this.edit.amount,
          description: this.edit.description, merchant: r.merchant, include: r.include,
        }, function () { self.editingId = null; self.loadRows(); });
      },

      patchRow: function (rowId, payload, done) {
        var self = this;
        fetch('/-/csv/v2/sessions/' + this.sessionId + '/rows/' + rowId, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
          .then(function (r) { if (!r.ok) throw new Error('edit failed'); return r.json(); })
          .then(function () { if (done) done(); })
          .catch(function () {
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Could not update row', type: 'error' } }));
          });
      },

      bulk: function (op, classification) {
        var self = this;
        fetch('/-/csv/v2/sessions/' + this.sessionId + '/bulk', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ op: op, classification: classification }),
        })
          .then(function (r) { if (!r.ok) throw new Error('bulk failed'); return r.json(); })
          .then(function () { self.loadRows(); })
          .catch(function () {
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Bulk action failed', type: 'error' } }));
          });
      },

      apply: function () {
        var self = this;
        this.loading = true;
        fetch('/-/csv/v2/sessions/' + this.sessionId + '/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: '{}',
        })
          .then(function (r) { return r.json().then(function (b) { return { ok: r.ok, body: b }; }); })
          .then(function (res) {
            self.loading = false;
            if (!res.ok) {
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: self.errText(res.body), type: 'error' } }));
              return;
            }
            self.result = res.body || {};
            self.step = 'result';
            var added = self.result.NewCount || 0;
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Imported ' + added + ' transaction' + (added === 1 ? '' : 's'), type: 'success' } }));
          })
          .catch(function () {
            self.loading = false;
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Import failed', type: 'error' } }));
          });
      },

      errText: function (body) {
        if (body && body.error && body.error.message) return body.error.message;
        return 'Request failed';
      },

      badgeTone: function (c) {
        return {
          'new': 'badge-success',
          'probable_dup': 'badge-warning',
          'exact_dup': 'badge-neutral',
          'conflict': 'badge-warning',
          'error': 'badge-error',
          'needs_account': 'badge-neutral',
        }[c] || 'badge-neutral';
      },

      classLabel: function (c) {
        return {
          'new': 'New',
          'probable_dup': 'Maybe dup',
          'exact_dup': 'Duplicate',
          'conflict': 'Conflict',
          'error': 'Error',
          'needs_account': '—',
        }[c] || c;
      },
    };
  });
});
