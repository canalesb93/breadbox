package pages

import (
	"encoding/json"
	"fmt"
)

// csvImportBootstrap renders the inline <script> that powers the CSV import
// 4-step wizard. Extracted from the templ template so the JS body stays
// plain text and doesn't compete with templ's `{ }` interpolation.
//
// Mirrors the original html/template version byte-for-byte. The four
// interpolation sites:
//   - csrfToken  ← {{.CSRFToken}}
//   - connectionID ← {{.ConnectionID}}
//   - existingUserID ← {{.ExistingUserID}}
//   - the {{.ExistingConnectionName}} / {{.ExistingUserName}} strings the
//     re-import branch of goToStep3 prints into the summary cells.
//
// The two `{{if .ConnectionID}}` / `{{if not .ConnectionID}}` blocks the
// original used to switch upload validation + summary fill are folded into
// runtime `if (connectionID)` checks here — the elements they guard
// (`csv-user-id`, `account-name`) only exist in the DOM in the matching
// branch, so the runtime check is equivalent.
func csvImportBootstrap(p CSVImportProps) string {
	jsonStr := func(s string) string {
		b, _ := json.Marshal(s)
		return string(b)
	}
	return fmt.Sprintf(`<script>
(function() {
  var csrfToken = %s;
  var connectionID = %s;
  var existingUserID = %s;
  var existingConnectionName = %s;
  var existingUserName = %s;
  var uploadData = null;
  var detectedDateFormat = "";
  var totalRows = 0;

  function showUploadError(msg) {
    var alert = document.getElementById("upload-alert");
    var status = document.getElementById("upload-status");
    status.textContent = msg;
    alert.classList.remove("hidden");
    if (typeof lucide !== 'undefined') lucide.createIcons();
  }

  function clearUploadError() {
    var alert = document.getElementById("upload-alert");
    var status = document.getElementById("upload-status");
    status.textContent = "";
    alert.classList.add("hidden");
  }

  window.uploadFile = function() {
    var fileInput = document.getElementById("csv-file");
    var progress = document.getElementById("upload-progress");
    var btn = document.getElementById("upload-btn");

    clearUploadError();

    if (!fileInput.files.length) {
      showUploadError("Please select a file.");
      return;
    }

    if (!connectionID) {
      var userSelect = document.getElementById("csv-user-id");
      if (!userSelect.value) {
        showUploadError("Please select a family member.");
        return;
      }
    }

    btn.disabled = true;
    btn.classList.add("btn-disabled");
    progress.textContent = "Uploading...";

    var formData = new FormData();
    formData.append("file", fileInput.files[0]);

    fetch("/-/csv/upload", {
      method: "POST",
      headers: {"X-CSRF-Token": csrfToken},
      body: formData
    })
    .then(function(res) { return res.json().then(function(d) { return {ok: res.ok, data: d}; }); })
    .then(function(res) {
      btn.disabled = false;
      btn.classList.remove("btn-disabled");
      progress.textContent = "";
      if (!res.ok) {
        showUploadError(res.data.error || "Upload failed.");
        return;
      }
      uploadData = res.data;
      totalRows = res.data.total_rows;
      setupStep2(res.data);
    })
    .catch(function() {
      btn.disabled = false;
      btn.classList.remove("btn-disabled");
      progress.textContent = "";
      showUploadError("Network error.");
    });
  };

  function setupStep2(data) {
    var headers = data.headers;
    var selects = ["map-date", "map-amount", "map-description", "map-category", "map-merchant", "map-debit", "map-credit"];
    var optionalSelects = ["map-category", "map-merchant", "map-debit", "map-credit"];

    selects.forEach(function(id) {
      var sel = document.getElementById(id);
      sel.innerHTML = "";
      if (optionalSelects.indexOf(id) !== -1) {
        var opt = document.createElement("option");
        opt.value = "-1";
        opt.textContent = "— None —";
        sel.appendChild(opt);
      }
      headers.forEach(function(h, i) {
        var opt = document.createElement("option");
        opt.value = i;
        opt.textContent = h;
        sel.appendChild(opt);
      });
    });

    // Apply detected columns.
    var dc = data.detected_columns || {};
    if (dc.date !== undefined) document.getElementById("map-date").value = dc.date;
    if (dc.amount !== undefined) document.getElementById("map-amount").value = dc.amount;
    if (dc.description !== undefined) document.getElementById("map-description").value = dc.description;
    if (dc.category !== undefined) document.getElementById("map-category").value = dc.category;
    if (dc.merchant_name !== undefined) document.getElementById("map-merchant").value = dc.merchant_name;
    if (dc.debit !== undefined) document.getElementById("map-debit").value = dc.debit;
    if (dc.credit !== undefined) document.getElementById("map-credit").value = dc.credit;

    if (data.template_name) {
      document.getElementById("template-detected").textContent = "Detected template: " + data.template_name;
    } else {
      document.getElementById("template-detected").textContent = "No template detected — please map columns manually.";
    }

    if (data.has_debit_credit) {
      document.getElementById("has-debit-credit").checked = true;
      toggleDebitCredit();
    }

    if (data.positive_is_debit !== undefined) {
      var radios = document.getElementsByName("sign-convention");
      radios[0].checked = data.positive_is_debit;
      radios[1].checked = !data.positive_is_debit;
    }

    if (data.date_format) {
      detectedDateFormat = data.date_format;
    }

    goToStep(2);
  }

  window.toggleDebitCredit = function() {
    var checked = document.getElementById("has-debit-credit").checked;
    var section = document.getElementById("debit-credit-section");
    if (checked) {
      section.classList.remove("hidden");
    } else {
      section.classList.add("hidden");
    }
  };

  window.previewData = function() {
    var status = document.getElementById("preview-status");
    status.textContent = "Loading preview...";

    var mapping = getColumnMapping();
    var positiveIsDebit = document.querySelector('input[name="sign-convention"]:checked').value === "debit";
    var hasDebitCredit = document.getElementById("has-debit-credit").checked;

    fetch("/-/csv/preview", {
      method: "POST",
      headers: {"Content-Type": "application/json", "X-CSRF-Token": csrfToken},
      body: JSON.stringify({
        column_mapping: mapping,
        positive_is_debit: positiveIsDebit,
        date_format: detectedDateFormat,
        has_debit_credit: hasDebitCredit
      })
    })
    .then(function(res) { return res.json().then(function(d) { return {ok: res.ok, data: d}; }); })
    .then(function(res) {
      if (!res.ok) {
        status.textContent = res.data.error || "Preview failed.";
        return;
      }
      status.textContent = "";
      if (res.data.date_format) {
        detectedDateFormat = res.data.date_format;
      }
      renderPreview(res.data.rows);
      document.getElementById("continue-btn").classList.remove("hidden");
    })
    .catch(function() {
      status.textContent = "Network error.";
    });
  };

  function renderPreview(rows) {
    var tbody = document.getElementById("preview-body");
    tbody.innerHTML = "";
    rows.forEach(function(row) {
      var tr = document.createElement("tr");
      if (row.error) tr.classList.add("bg-error/5");
      tr.innerHTML = "<td class='text-sm'>" + esc(row.date) + "</td>" +
        "<td class='text-sm tabular-nums'>" + esc(row.amount) + "</td>" +
        "<td class='text-sm'>" + esc(row.description) + "</td>" +
        "<td class='text-sm'>" + esc(row.category) + "</td>" +
        "<td class='text-sm'>" + esc(row.merchant) + "</td>" +
        "<td>" + (row.error ? "<span class='badge badge-error badge-sm'>" + esc(row.error) + "</span>" : "<span class='badge badge-success badge-sm'>OK</span>") + "</td>";
      tbody.appendChild(tr);
    });
    document.getElementById("preview-table-container").classList.remove("hidden");
  }

  function getColumnMapping() {
    var mapping = {};
    mapping.date = parseInt(document.getElementById("map-date").value, 10);
    mapping.description = parseInt(document.getElementById("map-description").value, 10);

    var hasDebitCredit = document.getElementById("has-debit-credit").checked;
    if (hasDebitCredit) {
      mapping.debit = parseInt(document.getElementById("map-debit").value, 10);
      mapping.credit = parseInt(document.getElementById("map-credit").value, 10);
    } else {
      mapping.amount = parseInt(document.getElementById("map-amount").value, 10);
    }

    var catVal = parseInt(document.getElementById("map-category").value, 10);
    if (catVal >= 0) mapping.category = catVal;

    var merchVal = parseInt(document.getElementById("map-merchant").value, 10);
    if (merchVal >= 0) mapping.merchant_name = merchVal;

    return mapping;
  }

  window.goToStep3 = function() {
    document.getElementById("summary-rows").textContent = totalRows;

    if (connectionID) {
      document.getElementById("summary-account").textContent = existingConnectionName;
      document.getElementById("summary-member").textContent = existingUserName;
    } else {
      var accountName = document.getElementById("account-name").value || "CSV Import";
      document.getElementById("summary-account").textContent = accountName;
      var userSelect = document.getElementById("csv-user-id");
      document.getElementById("summary-member").textContent = userSelect.options[userSelect.selectedIndex].text;
    }

    goToStep(3);
  };

  window.goToStep = function(n) {
    if (window._csvWizard) {
      window._csvWizard.setStep(n);
    }
    // Re-init Lucide icons for the new step
    setTimeout(function() { if (typeof lucide !== 'undefined') lucide.createIcons(); }, 50);
  };

  window.doImport = function() {
    var btn = document.getElementById("import-btn");
    var status = document.getElementById("import-status");
    btn.disabled = true;
    btn.classList.add("btn-disabled");
    status.textContent = "Importing...";

    var mapping = getColumnMapping();
    var positiveIsDebit = document.querySelector('input[name="sign-convention"]:checked').value === "debit";
    var hasDebitCredit = document.getElementById("has-debit-credit").checked;

    var body = {
      column_mapping: mapping,
      positive_is_debit: positiveIsDebit,
      date_format: detectedDateFormat,
      has_debit_credit: hasDebitCredit
    };

    if (connectionID) {
      body.connection_id = connectionID;
    } else {
      body.user_id = document.getElementById("csv-user-id").value;
      body.account_name = document.getElementById("account-name").value || "CSV Import";
    }

    fetch("/-/csv/import", {
      method: "POST",
      headers: {"Content-Type": "application/json", "X-CSRF-Token": csrfToken},
      body: JSON.stringify(body)
    })
    .then(function(res) { return res.json().then(function(d) { return {ok: res.ok, data: d}; }); })
    .then(function(res) {
      btn.disabled = false;
      btn.classList.remove("btn-disabled");
      if (!res.ok) {
        status.textContent = res.data.error || "Import failed.";
        return;
      }
      status.textContent = "";
      showResults(res.data);
    })
    .catch(function() {
      btn.disabled = false;
      btn.classList.remove("btn-disabled");
      status.textContent = "Network error.";
    });
  };

  function showResults(data) {
    document.getElementById("result-total").textContent = data.TotalRows;
    document.getElementById("result-new").textContent = data.NewCount;
    document.getElementById("result-updated").textContent = data.UpdatedCount;
    document.getElementById("result-skipped").textContent = data.SkippedCount;

    if (data.SkipReasons && data.SkipReasons.length > 0) {
      var list = document.getElementById("skip-reasons-list");
      list.innerHTML = "";
      data.SkipReasons.forEach(function(reason) {
        var li = document.createElement("li");
        li.textContent = reason;
        list.appendChild(li);
      });
      document.getElementById("skip-reasons").classList.remove("hidden");
    }

    document.getElementById("result-link").href = "/connections/" + data.ConnectionID;
    goToStep(4);
  }

  function esc(s) {
    if (!s) return "";
    var div = document.createElement("div");
    div.appendChild(document.createTextNode(s));
    return div.innerHTML;
  }

  // If re-importing, auto-show step 1 (which will redirect to upload).
  if (connectionID) {
    goToStep(1);
  }
})();
</script>`,
		jsonStr(p.CSRFToken),
		jsonStr(p.ConnectionID),
		jsonStr(p.ExistingUserID),
		jsonStr(p.ExistingConnectionName),
		jsonStr(p.ExistingUserName),
	)
}
