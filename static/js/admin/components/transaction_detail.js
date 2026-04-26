// Transaction detail Alpine components for /admin/transactions/{id}.
//
// Convention reference: docs/design-system.md -> "Alpine page components".
//
// The page renders three sibling Alpine factories (no parent root): the
// category editor sidebar, the tag manager, and the activity-timeline
// comment composer. Each factory takes no arguments; their initial state
// flows in via @templ.JSONScript and `data-*` attributes wired up in
// transaction_detail.templ.
//
// Three JSONScript payloads:
//   #transaction-detail-data            -> p.Categories (the two-level tree)
//   #transaction-detail-available-tags  -> p.AvailableTags
//   #transaction-detail-current-tags    -> p.CurrentTags
//
// Scalars flow via data-* attributes on each factory's root (see templ):
//   data-tx-id              -> p.TransactionID (all three factories)
//   data-category-override  -> p.Transaction.CategoryOverride ("true"/"false")
//   data-max-comment-length -> p.MaxCommentLength
//
// Back-compat note: the inline `categoryPicker` factory in tx_row.templ
// reads `window.__bbCategories` as a global. The page's category sidebar
// uses the same shared factory, so we seed `window.__bbCategories` from
// the JSONScript payload at module top-level (mirrors rule_detail.js).
// `window.__bbAllTags` is also seeded so the global tag picker (base.html)
// can open without a re-fetch.
//
// `showToast` is exposed on `window` to match the contract used by
// tx-row's inline x-init watcher and the rule_detail page.

// --- Module-level globals consumed by tx_row + base.html ---

function showToast(message, type) {
  window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
}

window.showToast = showToast;

// Seed window.__bbCategories and window.__bbAllTags as early as possible
// so any inline component (tx_row's categoryPicker, base.html's tag
// picker) that reads them on first render finds the populated values.
(function seedGlobals() {
  var catsEl = document.getElementById('transaction-detail-data');
  if (catsEl) {
    try {
      window.__bbCategories = JSON.parse(catsEl.textContent);
    } catch (e) {
      console.error('transactionDetail: failed to parse #transaction-detail-data', e);
    }
  }
  var tagsEl = document.getElementById('transaction-detail-available-tags');
  if (tagsEl) {
    try {
      window.__bbAllTags = JSON.parse(tagsEl.textContent) || [];
    } catch (e) {
      console.error('transactionDetail: failed to parse #transaction-detail-available-tags', e);
      window.__bbAllTags = [];
    }
  }
})();

document.addEventListener('alpine:init', function () {
  // Register transaction-detail scoped keyboard shortcuts in the global
  // registry (base.html). The dispatcher guards against touch, input
  // focus, and open overlays, so these handlers only need to care about
  // "what does this key do". Shortcuts surface automatically in the ?
  // help modal under "This page".
  var reg = Alpine.store('shortcuts');
  if (reg) {
    reg.register({
      id: 'transaction-detail.categorize',
      keys: 'c',
      description: 'Categorize transaction',
      group: 'Actions',
      scope: 'transaction-detail',
      action: function () {
        // Reuse the inline picker trigger - same contract as the click
        // handler, so the categoryPicker's sourceId + listener wiring stays
        // in one place.
        var btn = document.querySelector('.bb-cat-picker[data-picker-source="txd-cat"] button');
        if (btn) btn.click();
      },
    });

    reg.register({
      id: 'transaction-detail.tag',
      keys: 't',
      description: 'Edit tags',
      group: 'Actions',
      scope: 'transaction-detail',
      action: function () {
        // Click the "Add tag / Edit tags" chip so the tagManager component
        // opens the picker with its current tag state (appliedCounts, etc.).
        var btn = document.querySelector('.bb-tag-add');
        if (btn) btn.click();
      },
    });

    reg.register({
      id: 'transaction-detail.compose-note',
      keys: 'n',
      description: 'Add a note',
      group: 'Actions',
      scope: 'transaction-detail',
      action: function () {
        var el = document.getElementById('bb-txd-comment');
        if (!el) return;
        el.scrollIntoView({ behavior: 'smooth', block: 'center' });
        el.focus();
      },
    });

    reg.register({
      id: 'transaction-detail.expand-system-details',
      keys: 'e',
      description: 'Toggle system details',
      group: 'Actions',
      scope: 'transaction-detail',
      action: function () {
        var btn = document.getElementById('bb-txd-system-details-toggle');
        if (btn) btn.click();
      },
    });
  }

  // Category editor sidebar. Reads transaction id + override flag from
  // data-* attributes on its root.
  Alpine.data('txdCategoryEditor', function () {
    return {
      saving: false,
      isOverride: false,
      txId: '',

      init: function () {
        var ds = this.$el.dataset;
        this.txId = ds.txId || '';
        this.isOverride = ds.categoryOverride === 'true';
      },

      setCategoryFromPicker: function (detail) {
        if (!detail.id) {
          this.resetCategory();
          return;
        }
        var self = this;
        this.saving = true;
        fetch('/-/transactions/' + this.txId + '/category', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ category_id: detail.id }),
        })
          .then(function (r) {
            self.saving = false;
            if (r.ok) {
              self.isOverride = true;
              showToast('Category updated.', 'success');
            } else {
              r.json().then(function (d) { showToast(d.error?.message || 'Failed to update category.'); });
            }
          })
          .catch(function () { self.saving = false; showToast('Network error.'); });
      },

      resetCategory: function () {
        var self = this;
        this.saving = true;
        fetch('/-/transactions/' + this.txId + '/category', { method: 'DELETE' })
          .then(function (r) {
            self.saving = false;
            if (r.ok || r.status === 204) {
              self.isOverride = false;
              showToast('Category reset to auto-detected.', 'success');
              location.reload();
            } else {
              showToast('Failed to reset category.');
            }
          })
          .catch(function () { self.saving = false; showToast('Network error.'); });
      },
    };
  });

  // Tag manager (chip strip + Add/Edit picker). Reads transaction id from
  // a data-* attribute and the current/available tag lists from JSONScript.
  Alpine.data('txdTagManager', function () {
    return {
      tags: [],
      availableTags: [],
      txId: '',

      init: function () {
        this.txId = this.$el.dataset.txId || '';

        var currentEl = document.getElementById('transaction-detail-current-tags');
        var current = [];
        if (currentEl) {
          try {
            current = JSON.parse(currentEl.textContent) || [];
          } catch (e) {
            console.error('txdTagManager: failed to parse #transaction-detail-current-tags', e);
          }
        }
        this.tags = current.map(function (t) {
          return {
            slug: t.slug,
            displayName: t.display_name,
            color: t.color,
            icon: t.icon,
          };
        });

        // Re-use the global seeded by the page-level seedGlobals() block.
        this.availableTags = window.__bbAllTags || [];
      },

      openTagPicker: function () {
        // Picker uses the same contract as the bulk bar: the tx's current
        // tags render as "present" so the user can add and remove in one
        // commit.
        var counts = {};
        this.tags.forEach(function (t) { counts[t.slug] = 1; });
        window.dispatchEvent(new CustomEvent('open-tag-picker', {
          detail: {
            sourceId: 'txd-tag',
            transactionIds: [this.txId],
            txCount: 1,
            appliedCounts: counts,
            availableTags: this.availableTags,
          },
        }));
      },

      applyTagDiff: function (adds, removes) {
        adds = adds || [];
        removes = removes || [];
        if (adds.length + removes.length === 0) return;
        var op = {};
        if (adds.length) op.tags_to_add = adds.map(function (s) { return { slug: s, note: '' }; });
        if (removes.length) op.tags_to_remove = removes.map(function (s) { return { slug: s, note: '' }; });
        fetch('/-/transactions/batch-update', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            operations: [Object.assign({ transaction_id: this.txId }, op)],
            on_error: 'continue',
          }),
        })
          .then(function (r) { return r.json().then(function (d) { return { ok: r.ok || r.status === 422, body: d }; }); })
          .then(function (res) {
            if (res.ok && res.body && res.body.succeeded > 0) {
              showToast('Tags updated.', 'success');
              location.reload();
            } else {
              var msg = (res.body && res.body.error && res.body.error.message) || 'Failed to update tags.';
              showToast(msg);
            }
          })
          .catch(function () { showToast('Network error.'); });
      },

      removeTag: function (tag, note) {
        // Inline x on a tag chip - direct DELETE. Note is optional.
        var url = '/-/transactions/' + this.txId + '/tags/' + encodeURIComponent(tag.slug);
        fetch(url, {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ note: note || '' }),
        })
          .then(function (res) { return res.json().then(function (d) { return { ok: res.ok, body: d }; }); })
          .then(function (r) {
            if (r.ok) {
              showToast('Tag removed.', 'success');
              location.reload();
            } else {
              showToast(r.body.error || 'Failed to remove tag.');
            }
          })
          .catch(function () { showToast('Network error.'); });
      },
    };
  });

  // Activity-timeline comment composer. Reads txId + maxLength +
  // hasPendingReview from data-*. The `Toggle Needs Review` checkbox in the
  // composer card binds to `pinReview`; when posting, if pinReview is on
  // and the transaction doesn't already have a pending review, we also
  // POST a `needs-review` tag.
  Alpine.data('txdCommentManager', function () {
    return {
      newComment: '',
      maxLength: 10000,
      txId: '',
      hasPendingReview: false,
      pinReview: false,

      init: function () {
        var ds = this.$el.dataset;
        this.txId = ds.txId || '';
        var parsed = parseInt(ds.maxCommentLength, 10);
        if (!isNaN(parsed) && parsed > 0) this.maxLength = parsed;
        this.hasPendingReview = ds.hasPendingReview === 'true';
      },

      canSubmit: function () {
        var trimmed = this.newComment.trim().length;
        return trimmed > 0 && this.newComment.length <= this.maxLength;
      },

      counterState: function () {
        if (this.newComment.length === 0) return 'ok';
        var ratio = this.newComment.length / this.maxLength;
        if (ratio >= 1) return 'error';
        if (ratio >= 0.9) return 'warn';
        return 'ok';
      },

      autosize: function (el) {
        // Reset to natural height so scrollHeight reflects current content,
        // not the previously-set explicit height. Cap at max-h-36 (9rem
        // ~ 144px) so the composer never balloons past ~6 rows; overflow
        // scrolls inside.
        if (!el) return;
        el.style.height = 'auto';
        var max = 144;
        var next = Math.min(el.scrollHeight, max);
        el.style.height = next + 'px';
        el.style.overflowY = el.scrollHeight > max ? 'auto' : 'hidden';
      },

      addComment: function () {
        var self = this;
        var content = this.newComment.trim();
        if (!content || !this.canSubmit()) return;
        var shouldPin = this.pinReview && !this.hasPendingReview;
        fetch('/-/transactions/' + this.txId + '/comments', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content: content }),
        })
          .then(function (res) {
            if (!res.ok) {
              return res.json().then(function (data) {
                showToast(data.error || 'Failed to add comment.');
              });
            }
            if (!shouldPin) {
              self.newComment = '';
              showToast('Comment added.', 'success');
              location.reload();
              return;
            }
            return fetch('/-/transactions/' + self.txId + '/tags', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ slug: 'needs-review', note: '' }),
            })
              .then(function (tagRes) {
                self.newComment = '';
                if (tagRes.ok) {
                  showToast('Comment added; tagged needs-review.', 'success');
                } else {
                  showToast('Comment added (tag failed).', 'warning');
                }
                location.reload();
              })
              .catch(function () {
                self.newComment = '';
                showToast('Comment added (tag failed).', 'warning');
                location.reload();
              });
          })
          .catch(function () { showToast('Network error.'); });
      },

      deleteComment: function (id) {
        var self = this;
        bbConfirm({ title: 'Delete comment?', message: 'This comment will be permanently removed.', confirmLabel: 'Delete', variant: 'danger' }).then(function (ok) {
          if (!ok) return;
          fetch('/-/transactions/' + self.txId + '/comments/' + id, {
            method: 'DELETE',
          })
            .then(function (res) {
              if (res.ok || res.status === 204) {
                showToast('Comment deleted.', 'success');
                location.reload();
              } else {
                return res.json().then(function (data) {
                  showToast(data.error || 'Failed to delete comment.');
                });
              }
            })
            .catch(function () { showToast('Network error.'); });
        });
      },
    };
  });
});
