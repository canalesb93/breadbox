// CSV import wizard Alpine component for /admin/connections/import-csv.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
// The factory owns the wizard's `step` reactive state (replacing the old
// inline `x-data="{step: 1}"` block) and exposes window-level helpers that
// the existing inline `onclick="uploadFile()"` handlers in the templ
// markup continue to call. Step transitions go through `goToStep(n)` →
// this.setStep(n) so Alpine reacts and the `x-show="step === N"` blocks
// switch.
//
// Initial scalar state (CSRF token, optional re-import context) flows in
// through `data-*` attributes on the x-data root — see csv_import.templ.
// No JSON payload is needed; everything else is fetched from the server
// via the wizard's three endpoints (/-/csv/upload, /-/csv/preview,
// /-/csv/import).
//
// The original `_scripts.go` Go-string version interpolated the same five
// scalars via `fmt.Sprintf` + `json.Marshal`. Reading them off
// `this.$el.dataset` produces the same values without any Go-side string
// concatenation.

document.addEventListener('alpine:init', function () {
  Alpine.data('csvImport', function () {
    return {
      step: 1,

      // Read on init() and used by every fetch call below.
      csrfToken: '',
      connectionID: '',
      existingUserID: '',
      existingConnectionName: '',
      existingUserName: '',

      // Per-upload state. Reset every time setupStep2 runs.
      uploadData: null,
      detectedDateFormat: '',
      totalRows: 0,

      init: function () {
        var ds = this.$el.dataset;
        this.csrfToken = ds.csrfToken || '';
        this.connectionID = ds.connectionId || '';
        this.existingUserID = ds.existingUserId || '';
        this.existingConnectionName = ds.existingConnectionName || '';
        this.existingUserName = ds.existingUserName || '';

        var self = this;

        // The templ markup calls these via inline `onclick="..."` handlers.
        // Bind them to window so the existing markup still works without a
        // markup migration. Each shim closes over `self` (the Alpine
        // component instance) so it can mutate reactive state.
        window._csvWizard = { setStep: function (n) { self.setStep(n); } };
        window.uploadFile = function () { self.uploadFile(); };
        window.toggleDebitCredit = function () { self.toggleDebitCredit(); };
        window.previewData = function () { self.previewData(); };
        window.goToStep3 = function () { self.goToStep3(); };
        window.goToStep = function (n) { self.goToStep(n); };
        window.doImport = function () { self.doImport(); };

        // If re-importing, the original boot sequence forced step 1 (which
        // really just keeps the same default; preserved for parity).
        if (this.connectionID) {
          this.goToStep(1);
        }
      },

      setStep: function (n) {
        this.step = n;
      },

      goToStep: function (n) {
        this.setStep(n);
        // Re-init Lucide icons for the newly-visible step.
        setTimeout(function () {
          if (typeof lucide !== 'undefined') lucide.createIcons();
        }, 50);
      },

      showUploadError: function (msg) {
        var alert = document.getElementById('upload-alert');
        var status = document.getElementById('upload-status');
        status.textContent = msg;
        alert.classList.remove('hidden');
        if (typeof lucide !== 'undefined') lucide.createIcons();
      },

      clearUploadError: function () {
        var alert = document.getElementById('upload-alert');
        var status = document.getElementById('upload-status');
        status.textContent = '';
        alert.classList.add('hidden');
      },

      uploadFile: function () {
        var fileInput = document.getElementById('csv-file');
        var progress = document.getElementById('upload-progress');
        var btn = document.getElementById('upload-btn');

        this.clearUploadError();

        if (!fileInput.files.length) {
          this.showUploadError('Please select a file.');
          return;
        }

        if (!this.connectionID) {
          var userSelect = document.getElementById('csv-user-id');
          if (!userSelect.value) {
            this.showUploadError('Please select a family member.');
            return;
          }
        }

        btn.disabled = true;
        btn.classList.add('btn-disabled');
        progress.textContent = 'Uploading...';

        var formData = new FormData();
        formData.append('file', fileInput.files[0]);

        var self = this;
        fetch('/-/csv/upload', {
          method: 'POST',
          headers: { 'X-CSRF-Token': this.csrfToken },
          body: formData,
        })
          .then(function (res) { return res.json().then(function (d) { return { ok: res.ok, data: d }; }); })
          .then(function (res) {
            btn.disabled = false;
            btn.classList.remove('btn-disabled');
            progress.textContent = '';
            if (!res.ok) {
              self.showUploadError(res.data.error || 'Upload failed.');
              return;
            }
            self.uploadData = res.data;
            self.totalRows = res.data.total_rows;
            self.setupStep2(res.data);
          })
          .catch(function () {
            btn.disabled = false;
            btn.classList.remove('btn-disabled');
            progress.textContent = '';
            self.showUploadError('Network error.');
          });
      },

      setupStep2: function (data) {
        var headers = data.headers;
        var selects = ['map-date', 'map-amount', 'map-description', 'map-category', 'map-merchant', 'map-debit', 'map-credit'];
        var optionalSelects = ['map-category', 'map-merchant', 'map-debit', 'map-credit'];

        selects.forEach(function (id) {
          var sel = document.getElementById(id);
          sel.innerHTML = '';
          if (optionalSelects.indexOf(id) !== -1) {
            var opt = document.createElement('option');
            opt.value = '-1';
            opt.textContent = '— None —';
            sel.appendChild(opt);
          }
          headers.forEach(function (h, i) {
            var opt = document.createElement('option');
            opt.value = i;
            opt.textContent = h;
            sel.appendChild(opt);
          });
        });

        // Apply detected columns.
        var dc = data.detected_columns || {};
        if (dc.date !== undefined) document.getElementById('map-date').value = dc.date;
        if (dc.amount !== undefined) document.getElementById('map-amount').value = dc.amount;
        if (dc.description !== undefined) document.getElementById('map-description').value = dc.description;
        if (dc.category !== undefined) document.getElementById('map-category').value = dc.category;
        if (dc.merchant_name !== undefined) document.getElementById('map-merchant').value = dc.merchant_name;
        if (dc.debit !== undefined) document.getElementById('map-debit').value = dc.debit;
        if (dc.credit !== undefined) document.getElementById('map-credit').value = dc.credit;

        if (data.template_name) {
          document.getElementById('template-detected').textContent = 'Detected template: ' + data.template_name;
        } else {
          document.getElementById('template-detected').textContent = 'No template detected — please map columns manually.';
        }

        if (data.has_debit_credit) {
          document.getElementById('has-debit-credit').checked = true;
          this.toggleDebitCredit();
        }

        if (data.positive_is_debit !== undefined) {
          var radios = document.getElementsByName('sign-convention');
          radios[0].checked = data.positive_is_debit;
          radios[1].checked = !data.positive_is_debit;
        }

        if (data.date_format) {
          this.detectedDateFormat = data.date_format;
        }

        this.goToStep(2);
      },

      toggleDebitCredit: function () {
        var checked = document.getElementById('has-debit-credit').checked;
        var section = document.getElementById('debit-credit-section');
        if (checked) {
          section.classList.remove('hidden');
        } else {
          section.classList.add('hidden');
        }
      },

      previewData: function () {
        var status = document.getElementById('preview-status');
        status.textContent = 'Loading preview...';

        var mapping = this.getColumnMapping();
        var positiveIsDebit = document.querySelector('input[name="sign-convention"]:checked').value === 'debit';
        var hasDebitCredit = document.getElementById('has-debit-credit').checked;

        var self = this;
        fetch('/-/csv/preview', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrfToken },
          body: JSON.stringify({
            column_mapping: mapping,
            positive_is_debit: positiveIsDebit,
            date_format: this.detectedDateFormat,
            has_debit_credit: hasDebitCredit,
          }),
        })
          .then(function (res) { return res.json().then(function (d) { return { ok: res.ok, data: d }; }); })
          .then(function (res) {
            if (!res.ok) {
              status.textContent = res.data.error || 'Preview failed.';
              return;
            }
            status.textContent = '';
            if (res.data.date_format) {
              self.detectedDateFormat = res.data.date_format;
            }
            self.renderPreview(res.data.rows);
            document.getElementById('continue-btn').classList.remove('hidden');
          })
          .catch(function () {
            status.textContent = 'Network error.';
          });
      },

      renderPreview: function (rows) {
        var tbody = document.getElementById('preview-body');
        tbody.innerHTML = '';
        rows.forEach(function (row) {
          var tr = document.createElement('tr');
          if (row.error) tr.classList.add('bg-error/5');
          tr.innerHTML =
            "<td class='text-sm'>" + esc(row.date) + '</td>' +
            "<td class='text-sm tabular-nums'>" + esc(row.amount) + '</td>' +
            "<td class='text-sm'>" + esc(row.description) + '</td>' +
            "<td class='text-sm'>" + esc(row.category) + '</td>' +
            "<td class='text-sm'>" + esc(row.merchant) + '</td>' +
            '<td>' +
            (row.error
              ? "<span class='badge badge-error badge-sm'>" + esc(row.error) + '</span>'
              : "<span class='badge badge-success badge-sm'>OK</span>") +
            '</td>';
          tbody.appendChild(tr);
        });
        document.getElementById('preview-table-container').classList.remove('hidden');
      },

      getColumnMapping: function () {
        var mapping = {};
        mapping.date = parseInt(document.getElementById('map-date').value, 10);
        mapping.description = parseInt(document.getElementById('map-description').value, 10);

        var hasDebitCredit = document.getElementById('has-debit-credit').checked;
        if (hasDebitCredit) {
          mapping.debit = parseInt(document.getElementById('map-debit').value, 10);
          mapping.credit = parseInt(document.getElementById('map-credit').value, 10);
        } else {
          mapping.amount = parseInt(document.getElementById('map-amount').value, 10);
        }

        var catVal = parseInt(document.getElementById('map-category').value, 10);
        if (catVal >= 0) mapping.category = catVal;

        var merchVal = parseInt(document.getElementById('map-merchant').value, 10);
        if (merchVal >= 0) mapping.merchant_name = merchVal;

        return mapping;
      },

      goToStep3: function () {
        document.getElementById('summary-rows').textContent = this.totalRows;

        if (this.connectionID) {
          document.getElementById('summary-account').textContent = this.existingConnectionName;
          document.getElementById('summary-member').textContent = this.existingUserName;
        } else {
          var accountName = document.getElementById('account-name').value || 'CSV Import';
          document.getElementById('summary-account').textContent = accountName;
          var userSelect = document.getElementById('csv-user-id');
          document.getElementById('summary-member').textContent = userSelect.options[userSelect.selectedIndex].text;
        }

        this.goToStep(3);
      },

      doImport: function () {
        var btn = document.getElementById('import-btn');
        var status = document.getElementById('import-status');
        btn.disabled = true;
        btn.classList.add('btn-disabled');
        status.textContent = 'Importing...';

        var mapping = this.getColumnMapping();
        var positiveIsDebit = document.querySelector('input[name="sign-convention"]:checked').value === 'debit';
        var hasDebitCredit = document.getElementById('has-debit-credit').checked;

        var body = {
          column_mapping: mapping,
          positive_is_debit: positiveIsDebit,
          date_format: this.detectedDateFormat,
          has_debit_credit: hasDebitCredit,
        };

        if (this.connectionID) {
          body.connection_id = this.connectionID;
        } else {
          body.user_id = document.getElementById('csv-user-id').value;
          body.account_name = document.getElementById('account-name').value || 'CSV Import';
        }

        var self = this;
        fetch('/-/csv/import', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': this.csrfToken },
          body: JSON.stringify(body),
        })
          .then(function (res) { return res.json().then(function (d) { return { ok: res.ok, data: d }; }); })
          .then(function (res) {
            btn.disabled = false;
            btn.classList.remove('btn-disabled');
            if (!res.ok) {
              status.textContent = res.data.error || 'Import failed.';
              return;
            }
            status.textContent = '';
            self.showResults(res.data);
          })
          .catch(function () {
            btn.disabled = false;
            btn.classList.remove('btn-disabled');
            status.textContent = 'Network error.';
          });
      },

      showResults: function (data) {
        document.getElementById('result-total').textContent = data.TotalRows;
        document.getElementById('result-new').textContent = data.NewCount;
        document.getElementById('result-updated').textContent = data.UpdatedCount;
        document.getElementById('result-skipped').textContent = data.SkippedCount;

        if (data.SkipReasons && data.SkipReasons.length > 0) {
          var list = document.getElementById('skip-reasons-list');
          list.innerHTML = '';
          data.SkipReasons.forEach(function (reason) {
            var li = document.createElement('li');
            li.textContent = reason;
            list.appendChild(li);
          });
          document.getElementById('skip-reasons').classList.remove('hidden');
        }

        document.getElementById('result-link').href = '/connections/' + data.ConnectionID;
        this.goToStep(4);
      },
    };
  });
});

// Module-private HTML escaper used by renderPreview's string concatenation.
function esc(s) {
  if (!s) return '';
  var div = document.createElement('div');
  div.appendChild(document.createTextNode(s));
  return div.innerHTML;
}
