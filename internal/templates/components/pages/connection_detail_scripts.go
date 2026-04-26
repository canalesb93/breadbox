package pages

import (
	"fmt"
	"strings"
)

// connDetailErrorMessage maps Plaid/Teller error codes to human-readable
// messages. Mirrors the funcMap "errorMessage" helper in admin/templates.go.
func connDetailErrorMessage(code string) string {
	switch code {
	case "ITEM_LOGIN_REQUIRED":
		return "Your bank login has changed. Please re-authenticate."
	case "INSUFFICIENT_CREDENTIALS":
		return "Additional credentials are needed. Please re-authenticate."
	case "INVALID_CREDENTIALS":
		return "Your bank credentials are incorrect. Please re-authenticate."
	case "MFA_NOT_SUPPORTED":
		return "This connection requires MFA which is not supported. Please reconnect."
	case "NO_ACCOUNTS":
		return "No accounts found for this connection."
	case "enrollment.disconnected":
		return "This bank connection has been disconnected."
	}
	return code
}

// connDetailHeaderTileBg returns the icon-tile background class by provider.
func connDetailHeaderTileBg(provider string) string {
	switch provider {
	case "plaid":
		return "bg-info/10"
	case "teller":
		return "bg-success/10"
	default:
		return "bg-warning/10"
	}
}

// connDetailStatusBadge returns the inline HTML for the status badge.
// Mirrors the funcMap "statusBadge" helper in admin/templates.go. Returned
// as a string for use with @templ.Raw to preserve the literal markup.
func connDetailStatusBadge(status string) string {
	switch status {
	case "active":
		return `<span class="badge badge-soft badge-success badge-sm">Active</span>`
	case "pending_reauth":
		return `<span class="badge badge-soft badge-warning badge-sm">Reauth Needed</span>`
	case "error":
		return `<span class="badge badge-soft badge-error badge-sm">Error</span>`
	default:
		return `<span class="badge badge-ghost badge-sm">Disconnected</span>`
	}
}

// connDetailSuccessRateClass returns the success-rate text color class.
func connDetailSuccessRateClass(rate float64) string {
	switch {
	case rate >= 90:
		return "text-success"
	case rate >= 50:
		return "text-warning"
	default:
		return "text-error"
	}
}

// connDetailAvgDuration renders the avg-duration figure. Mirrors the
// nested template branches in the original markup so the dash-fallback
// span survives byte-for-byte. Emitted via @templ.Raw because it can
// produce literal &mdash;.
func connDetailAvgDuration(sec float64) string {
	switch {
	case sec > 60:
		return fmt.Sprintf("%.0fm", sec/60.0)
	case sec > 1:
		return fmt.Sprintf("%.1fs", sec)
	case sec > 0:
		return fmt.Sprintf("%.0fms", sec*1000.0)
	default:
		return `<span class="text-base-content/30">&mdash;</span>`
	}
}

// connDetailBarStyle returns the inline style attribute value for a
// success-or-error bar in the 14-day timeline. Mirrors the original
// `style="height: <pct>px; min-height: 4px;"` calculation. `mine` is the
// count for this color, `other` is the count for the other color, and
// total is mine+other (or the row total).
func connDetailBarStyle(mine, other, total int) string {
	if other > 0 {
		// Pixel-equivalent height (52 * mine / total) so the two bars
		// add up to 52px when both colors are present.
		px := (float64(mine) * 52.0) / float64(total)
		return fmt.Sprintf("height: %.0fpx; min-height: 4px;", px)
	}
	return "height: 52px; min-height: 4px;"
}

// connDetailAccentBar returns the colored accent-bar class for an account
// card (left edge). Mirrors the source map of account types to bg colors.
func connDetailAccentBar(t string) string {
	switch t {
	case "depository":
		return "bg-info/50"
	case "credit":
		return "bg-warning/50"
	case "loan":
		return "bg-error/40"
	case "investment":
		return "bg-success/50"
	default:
		return "bg-base-300/50"
	}
}

// connDetailAccountTileBg returns the icon-tile background class for an
// account card.
func connDetailAccountTileBg(t string) string {
	switch t {
	case "depository":
		return "bg-info/8"
	case "credit":
		return "bg-warning/8"
	case "loan":
		return "bg-error/8"
	case "investment":
		return "bg-success/8"
	default:
		return "bg-base-200/60"
	}
}

// connDetailAccountIcon returns the lucide icon name for an account type.
// Mirrors the funcMap "accountTypeIcon" helper.
func connDetailAccountIcon(t string) string {
	switch t {
	case "depository":
		return "landmark"
	case "credit":
		return "credit-card"
	case "loan":
		return "file-text"
	case "investment":
		return "trending-up"
	default:
		return "wallet"
	}
}

// connDetailAccountIconColor returns the icon color class for an account type.
func connDetailAccountIconColor(t string) string {
	switch t {
	case "depository":
		return "text-info/60"
	case "credit":
		return "text-warning/70"
	case "loan":
		return "text-error/60"
	case "investment":
		return "text-success/60"
	default:
		return "text-base-content/40"
	}
}

// connDetailBalanceColor returns the balance text color class — credit/loan
// types render warning/error tints; everything else uses default.
func connDetailBalanceColor(t string) string {
	switch t {
	case "credit":
		return "text-warning/80"
	case "loan":
		return "text-error/80"
	default:
		return ""
	}
}

// connDetailAccountTypeLabel returns the human-readable label for an
// account's type/subtype combination. Mirrors the funcMap
// "accountTypeLabel" helper.
func connDetailAccountTypeLabel(acctType, subtype string, subtypeValid bool) string {
	if subtypeValid && subtype != "" {
		labels := map[string]string{
			"checking":     "Checking",
			"savings":      "Savings",
			"credit card":  "Credit Card",
			"credit_card":  "Credit Card",
			"money market": "Money Market",
			"money_market": "Money Market",
			"cd":           "CD",
			"paypal":       "PayPal",
			"student":      "Student Loan",
			"mortgage":     "Mortgage",
			"auto":         "Auto Loan",
			"401k":         "401(k)",
			"ira":          "IRA",
			"brokerage":    "Brokerage",
			"prepaid":      "Prepaid",
			"hsa":          "HSA",
		}
		if label, ok := labels[subtype]; ok {
			return label
		}
		return subtype
	}
	labels := map[string]string{
		"depository": "Bank Account",
		"credit":     "Credit Card",
		"loan":       "Loan",
		"investment": "Investment",
	}
	if label, ok := labels[acctType]; ok {
		return label
	}
	return acctType
}

// connDetailSyncIconBg returns the icon-circle background class for a sync log row.
func connDetailSyncIconBg(status string) string {
	switch status {
	case "success":
		return "bg-success/12"
	case "error":
		return "bg-error/12"
	default:
		return "bg-base-200"
	}
}

// connDetailErrTitle returns the title= attribute for the inline error
// paragraph (raw error string when available).
func connDetailErrTitle(sl SyncLogRow) string {
	if sl.ErrorMessageValid {
		return sl.ErrorMessageString
	}
	return ""
}

// connDetailDisplayNameInput renders the per-account display-name <input>
// with its inline onchange handler. Templ's `on*` attribute slots reject
// raw strings, so this helper emits the element verbatim and the templ
// page injects it via @templ.Raw — same effect as the original
// html/template `<input ... onchange="updateDisplayName(...)">` row.
func connDetailDisplayNameInput(a AccountRow) string {
	return fmt.Sprintf(
		`<input type="text" value=%q placeholder="Display name..." onchange="updateDisplayName('%s', this.value)" class="input input-xs input-ghost flex-1 hover:input-bordered focus:input-bordered transition-all rounded-lg text-xs">`,
		htmlAttr(a.DisplayName), htmlEscapeJS(a.ID),
	)
}

// connDetailExcludeCheckbox renders the per-account exclude checkbox with
// its inline onchange handler. Same rationale as connDetailDisplayNameInput.
func connDetailExcludeCheckbox(a AccountRow) string {
	checked := ""
	if a.Excluded {
		checked = " checked"
	}
	return fmt.Sprintf(
		`<input type="checkbox" class="checkbox checkbox-xs"%s onchange="toggleExcluded('%s', this.checked)">`,
		checked, htmlEscapeJS(a.ID),
	)
}

// htmlAttr is a tiny escape helper for double-quoted HTML attribute
// values. Account IDs are short_id/UUIDs so this is mostly belt-and-
// suspenders, but display names are user-controlled.
func htmlAttr(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		`"`, "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}

// htmlEscapeJS escapes a value for use inside a single-quoted JS string
// literal that lives inside an HTML attribute. Account IDs are 8-char
// base62 or UUID hex — trivially safe — but normalize here so the helper
// can be reused for anything else.
func htmlEscapeJS(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
	)
	return r.Replace(s)
}

// connDetailScripts emits the page's <script> block. Lifted verbatim from
// the original connection_detail.html with the two `{{.ConnID}}` template
// substitutions replaced by sprintf interpolation. Returned as a string
// for use with @templ.Raw — keeps the template declarative and the JS
// body byte-identical.
func connDetailScripts(p ConnectionDetailProps) string {
	const body = `<script>
function showToast(message, type) {
  window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
}

var SYNC_POLL_INTERVAL_MS = 1500;
var SYNC_POLL_MAX_MS = 5 * 60 * 1000;
var syncPollTimer = null;
var syncPollStartedAt = 0;

function setSyncButtonIdle() {
  var btn = document.getElementById('sync-btn');
  if (!btn) return;
  btn.disabled = false;
  btn.innerHTML = '<i data-lucide="refresh-cw" class="w-3.5 h-3.5"></i> Sync Now';
  lucide.createIcons({nodes: [btn]});
}

function setSyncButtonBusy() {
  var btn = document.getElementById('sync-btn');
  if (!btn) return;
  btn.disabled = true;
  btn.innerHTML = '<span class="loading loading-spinner loading-xs"></span> Syncing...';
}

function buildSyncLogRow(payload) {
  var row = document.createElement('div');
  row.className = 'flex gap-3 py-3 border-b border-base-300/30 last:border-0';
  row.setAttribute('data-sync-log-row', payload.short_id || 'pending');
  row.setAttribute('data-sync-log-status', payload.status || 'in_progress');
  row.innerHTML = ''
    + '<div class="flex flex-col items-center shrink-0 pt-0.5" data-sync-log-icon>'
    +   '<div class="w-6 h-6 rounded-full flex items-center justify-center bg-base-200">'
    +     '<span class="loading loading-spinner loading-xs text-primary"></span>'
    +   '</div>'
    + '</div>'
    + '<div class="flex-1 min-w-0 flex items-center justify-between gap-2">'
    +   '<div class="min-w-0">'
    +     '<div class="flex items-center gap-2 flex-wrap">'
    +       '<span class="text-sm font-medium capitalize"></span>'
    +       '<span class="text-xs text-base-content/35 tabular-nums" data-sync-log-time>just now</span>'
    +       '<span class="text-[0.6rem] text-base-content/25 tabular-nums bg-base-200/50 px-1.5 py-0.5 rounded-full hidden" data-sync-log-duration></span>'
    +     '</div>'
    +     '<p class="text-xs text-error/60 mt-0.5 truncate hidden" data-sync-log-error></p>'
    +   '</div>'
    +   '<div class="flex items-center gap-2.5 tabular-nums text-xs shrink-0" data-sync-log-counts>'
    +     '<div class="flex items-center gap-1.5 hidden" data-sync-log-counts-main>'
    +       '<span class="text-success font-medium hidden" data-sync-log-added></span>'
    +       '<span class="text-info font-medium hidden" data-sync-log-modified></span>'
    +       '<span class="text-error font-medium hidden" data-sync-log-removed></span>'
    +     '</div>'
    +     '<span class="text-base-content/25 text-[0.6rem] hidden" data-sync-log-unchanged></span>'
    +   '</div>'
    + '</div>';
  row.querySelector('span.capitalize').textContent = payload.trigger || 'manual';
  return row;
}

function toggleHidden(el, hidden) {
  if (!el) return;
  if (hidden) el.classList.add('hidden'); else el.classList.remove('hidden');
}

function updateSyncLogRow(row, payload) {
  if (!row) return;
  var status = payload.status || 'in_progress';
  row.setAttribute('data-sync-log-status', status);
  if (payload.short_id) row.setAttribute('data-sync-log-row', payload.short_id);

  var iconWrap = row.querySelector('[data-sync-log-icon] > div');
  if (iconWrap) {
    var iconHTML, iconClass;
    if (status === 'success') {
      iconClass = 'bg-success/12';
      iconHTML = '<i data-lucide="check" class="w-3 h-3 text-success"></i>';
    } else if (status === 'error') {
      iconClass = 'bg-error/12';
      iconHTML = '<i data-lucide="x" class="w-3 h-3 text-error"></i>';
    } else {
      iconClass = 'bg-base-200';
      iconHTML = '<span class="loading loading-spinner loading-xs text-primary"></span>';
    }
    iconWrap.className = 'w-6 h-6 rounded-full flex items-center justify-center ' + iconClass;
    iconWrap.innerHTML = iconHTML;
    if (status === 'success' || status === 'error') lucide.createIcons({nodes: [iconWrap]});
  }

  var dur = row.querySelector('[data-sync-log-duration]');
  if (dur) {
    if (payload.duration_label) {
      dur.textContent = payload.duration_label;
      toggleHidden(dur, false);
    } else {
      toggleHidden(dur, true);
    }
  }

  var err = row.querySelector('[data-sync-log-error]');
  if (err) {
    if (status === 'error' && (payload.friendly_error_message || payload.error_message)) {
      err.textContent = payload.friendly_error_message || payload.error_message;
      err.title = payload.error_message || '';
      toggleHidden(err, false);
    } else {
      toggleHidden(err, true);
    }
  }

  var added = payload.added_count || 0;
  var modified = payload.modified_count || 0;
  var removed = payload.removed_count || 0;
  var unchanged = payload.unchanged_count || 0;
  var addedEl = row.querySelector('[data-sync-log-added]');
  var modEl = row.querySelector('[data-sync-log-modified]');
  var remEl = row.querySelector('[data-sync-log-removed]');
  var unchEl = row.querySelector('[data-sync-log-unchanged]');
  var main = row.querySelector('[data-sync-log-counts-main]');
  if (addedEl) { addedEl.textContent = added ? '+' + added : ''; toggleHidden(addedEl, !added); }
  if (modEl)   { modEl.textContent   = modified ? '~' + modified : ''; toggleHidden(modEl, !modified); }
  if (remEl)   { remEl.textContent   = removed ? '-' + removed : ''; toggleHidden(remEl, !removed); }
  if (unchEl)  { unchEl.textContent  = unchanged ? '=' + unchanged : ''; unchEl.title = unchanged + ' unchanged'; toggleHidden(unchEl, !unchanged); }
  toggleHidden(main, !(added || modified || removed));
}

function findPollTargetRow() {
  var list = document.getElementById('sync-history-list');
  if (!list) return null;
  var pending = list.querySelector('[data-sync-log-row="pending"]');
  if (pending) return pending;
  var first = list.querySelector('[data-sync-log-row]');
  if (first && first.getAttribute('data-sync-log-status') === 'in_progress') return first;
  return null;
}

function stopSyncPoll() {
  if (syncPollTimer) { clearTimeout(syncPollTimer); syncPollTimer = null; }
}

function pollSyncStatus() {
  stopSyncPoll();
  if (!syncPollStartedAt) syncPollStartedAt = Date.now();

  fetch('/-/connections/%[1]s/sync-status', { headers: { 'Accept': 'application/json' } })
    .then(function(res) { return res.ok ? res.json() : null; })
    .then(function(payload) {
      if (!payload || payload.status === 'none') {
        syncPollTimer = setTimeout(pollSyncStatus, SYNC_POLL_INTERVAL_MS);
        return;
      }

      var row = findPollTargetRow();
      if (row) updateSyncLogRow(row, payload);

      if (payload.status === 'in_progress') {
        if (Date.now() - syncPollStartedAt > SYNC_POLL_MAX_MS) {
          showToast('Sync is taking longer than expected — refresh to check status.', 'warning');
          syncPollStartedAt = 0;
          setSyncButtonIdle();
          return;
        }
        syncPollTimer = setTimeout(pollSyncStatus, SYNC_POLL_INTERVAL_MS);
        return;
      }

      syncPollStartedAt = 0;
      setSyncButtonIdle();
      if (payload.status === 'error') {
        showToast(payload.friendly_error_message || payload.error_message || 'Sync failed.', 'error');
      }
    })
    .catch(function() {
      syncPollTimer = setTimeout(pollSyncStatus, SYNC_POLL_INTERVAL_MS);
    });
}

function syncConnection() {
  setSyncButtonBusy();

  var list = document.getElementById('sync-history-list');
  var empty = document.getElementById('sync-history-empty');
  if (list && !list.querySelector('[data-sync-log-row="pending"]')) {
    var pending = buildSyncLogRow({ status: 'in_progress', trigger: 'manual' });
    if (list.firstElementChild) list.insertBefore(pending, list.firstElementChild);
    else list.appendChild(pending);
    toggleHidden(list, false);
    toggleHidden(empty, true);
  }

  fetch('/-/connections/%[1]s/sync', { method: 'POST' })
    .then(function(res) {
      if (res.ok) {
        syncPollStartedAt = Date.now();
        pollSyncStatus();
        return;
      }
      return res.json().then(function(data) {
        showToast((data && data.error) || 'Failed to trigger sync.');
        var p = list && list.querySelector('[data-sync-log-row="pending"]');
        if (p) p.remove();
        if (list && !list.firstElementChild) { toggleHidden(list, true); toggleHidden(empty, false); }
        setSyncButtonIdle();
      });
    })
    .catch(function() {
      showToast('Network error. Please try again.');
      var p = list && list.querySelector('[data-sync-log-row="pending"]');
      if (p) p.remove();
      if (list && !list.firstElementChild) { toggleHidden(list, true); toggleHidden(empty, false); }
      setSyncButtonIdle();
    });
}

document.addEventListener('DOMContentLoaded', function() {
  var first = document.querySelector('#sync-history-list [data-sync-log-row]');
  if (first && first.getAttribute('data-sync-log-status') === 'in_progress') {
    setSyncButtonBusy();
    pollSyncStatus();
  }
});

function removeConnection() {
  if (window.bbProgress) window.bbProgress.start();
  fetch("/-/connections/%[1]s", { method: "DELETE" })
  .then(function (res) {
    if (res.ok) {
      if (window.bbProgress) window.bbProgress.finish();
      window.location.href = "/connections";
    } else {
      if (window.bbProgress) window.bbProgress.finish();
      return res.json().then(function (data) {
        showToast(data.error || "Failed to remove connection.");
      });
    }
  })
  .catch(function () {
    if (window.bbProgress) window.bbProgress.finish();
    showToast("Network error. Please try again.");
  });
}

function togglePause() {
  var btn = document.getElementById('pause-btn');
  var isPaused = btn.textContent.trim() === 'Resume';
  var newPaused = !isPaused;
  btn.disabled = true;
  fetch("/-/connections/%[1]s/paused", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({paused: newPaused})
  })
  .then(function(res) {
    if (res.ok) {
      window.location.reload();
    } else {
      return res.json().then(function(data) {
        showToast(data.error || "Failed to update pause state.");
        btn.disabled = false;
      });
    }
  })
  .catch(function() {
    showToast("Network error. Please try again.");
    btn.disabled = false;
  });
}

function updateSyncInterval(val) {
  var body = val === "" ? {minutes: null} : {minutes: parseInt(val, 10)};
  var status = document.getElementById('interval-status');
  status.textContent = 'Saving...';
  fetch("/-/connections/%[1]s/sync-interval", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(body)
  })
  .then(function(res) {
    if (res.ok) {
      status.textContent = 'Saved';
      setTimeout(function() { status.textContent = ''; }, 2000);
    } else {
      return res.json().then(function(data) {
        status.textContent = data.error || 'Failed';
      });
    }
  })
  .catch(function() {
    status.textContent = 'Network error';
  });
}

function updateDisplayName(accountId, val) {
  var body = val === "" ? {display_name: null} : {display_name: val};
  fetch("/-/accounts/" + accountId + "/display-name", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(body)
  })
  .then(function(res) {
    if (res.ok) {
      showToast("Display name updated.", "success");
    } else {
      return res.json().then(function(data) {
        showToast(data.error || "Failed to update display name.");
      });
    }
  })
  .catch(function() {
    showToast("Network error. Please try again.");
  });
}

function toggleExcluded(accountId, checked) {
  fetch("/-/accounts/" + accountId + "/excluded", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({excluded: checked})
  })
  .then(function(res) {
    if (res.ok) {
      showToast(checked ? "Account excluded." : "Account included.", "success");
      window.location.reload();
    } else {
      return res.json().then(function(data) {
        showToast(data.error || "Failed to update excluded state.");
      });
    }
  })
  .catch(function() {
    showToast("Network error. Please try again.");
  });
}
</script>`
	// jsEscape connID to be safe in fetch URLs (already a UUID string from
	// chi, but encodeURIComponent the JS-side equivalent is safer).
	connID := strings.ReplaceAll(p.ConnID, "\"", "")
	return fmt.Sprintf(body, connID)
}
