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
//
// ---- Optimistic in-place activity-timeline updates (Strategy A) ----
//
// Mutating actions (set category, add/remove tag, add/delete comment,
// pin/unpin needs-review) used to call location.reload() — or, in the
// category-set case, do nothing visible — to surface the resulting
// timeline row. We now keep the user on the page:
//
//   1. POST/PATCH/DELETE the mutation as before.
//   2. On 2xx, GET /-/transactions/{id}/timeline/rows?since=<lastTs>.
//      The server reuses the same templ helpers as the main page (see
//      pages.TimelineRows in transaction_detail.templ) so row markup is
//      a single source of truth — no client-side row templating, no
//      drift risk from a duplicated JS renderer.
//   3. Insert the returned <li> rows just before the composer at the
//      bottom of the timeline <ol>. Update the cached "last activity
//      timestamp" cursor so subsequent fetches only get fresh rows.
//   4. Update local chip state (category chip, tag chips, pin state) so
//      the sidebar reflects the change without a reload.
//
// Failure path: every catch / non-2xx branch calls restorePageState()
// (clears the SPA progress bar + content fade left over from any link
// click) and surfaces a toast. Chip-state rollback happens at the call
// site so the user sees the prior state restored.
//
// Day-grouping: the render endpoint inserts a `<li class="...">Today</li>`
// separator ahead of the new rows when the new rows fall on a different
// calendar day than the most recent existing row (see TimelineRowsHandler
// in internal/admin/transactions.go).
//
// ---- Cross-factory needs-review sync (Option B: $dispatch event) ----
//
// The composer's "Add Needs Review" affordance is bound to
// `pinReview` and gated by `hasPendingReview` inside txdCommentManager,
// while the timeline's tag chips (add via picker, remove via inline ✕)
// live in txdTagManager. They are sibling Alpine factories with no
// shared store, so a successful timeline-driven mutation of the
// `needs-review` slug used to leave the composer's toggle stuck on its
// old state — e.g. removing the chip from the timeline kept the
// composer reading "Needs review tagged" until a full reload.
//
// Fix: when a timeline-driven add/remove of the `needs-review` slug
// confirms server-side, dispatch a `bb-needs-review-changed` window
// event with `{ isOn: bool }`. txdCommentManager's init() listens and
// flips `hasPendingReview` (and clears `pinReview` when the tag is
// already present) so the composer re-renders against the new truth.
// Slug-scoped — other tag mutations don't touch the composer.

// --- Module-level globals consumed by tx_row + base.html ---

function showToast(message, type) {
  window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: message, type: type || 'error' } }));
}

window.showToast = showToast;

// restorePageState clears the global SPA progress bar + content fade that
// auto-starts on internal link clicks. Per .claude/rules/ui.md every
// async error path must invoke this so the page doesn't stay blurred /
// non-interactive after a failed fetch. Defined at module scope so all
// three factories can share a single implementation.
function restorePageState() {
  if (window.bbProgress && typeof window.bbProgress.finish === 'function') {
    window.bbProgress.finish();
  }
  var main = document.querySelector('main');
  if (main) {
    main.style.opacity = '';
    main.style.filter = '';
    main.style.pointerEvents = '';
  }
}

// timelineRoot returns the activity <section> element (root of the
// txdCommentManager factory). Used by the cross-factory helpers below
// because the comment manager owns the cursor + insertion point but
// every factory needs to trigger a refresh after its mutation.
function timelineRoot() {
  return document.getElementById('activity');
}

// timelineList returns the <ol class="bb-timeline"> inside the activity
// section (or null when the timeline started empty — only the
// txdTimelineEmptyComposer was rendered).
function timelineList() {
  var root = timelineRoot();
  if (!root) return null;
  return root.querySelector('ol.bb-timeline');
}

// composerLi finds the <li> wrapping txdComposerCard at the bottom of
// the timeline. New rows must insert *before* this so the composer
// stays at the bottom.
function composerLi() {
  var ol = timelineList();
  if (!ol) return null;
  // The composer is the last child <li>. Use lastElementChild instead
  // of a class selector because the composer's <li> doesn't carry a
  // distinguishing class today.
  return ol.lastElementChild;
}

// fetchTimelineRows fetches and inserts the rendered <li> rows for any
// activity entries newer than the cached cursor. The optional
// `replaceCommentIDs` array forces the endpoint to also return rows for
// those comment short_ids regardless of cursor age — used by the
// soft-delete path where the tombstone replaces an existing bubble (its
// CreatedAt is older than `since`, but `is_deleted` just flipped).
//
// The optional `opts` object controls post-insert scroll behavior:
//   { anchorComposer: true } — pin the composer to its pre-insertion
//     viewport y-position instead of scrolling the new row into view.
//     Used by the comment-add path so the textarea the user was just
//     typing into doesn't get pushed off-screen by the optimistic row.
//     Other actions (category set, tag add) leave this off and keep the
//     default "scroll new row into view" behavior so the user sees the
//     change land.
//
// Returns a promise that resolves to the number of rows inserted (0 = no
// new rows, treat as silent success).
function fetchTimelineRows(txId, replaceCommentIDs, opts) {
  var root = timelineRoot();
  if (!root) return Promise.resolve(0);
  var since = root.dataset.lastActivityTs || '';
  var params = [];
  if (since) params.push('since=' + encodeURIComponent(since));
  if (replaceCommentIDs && replaceCommentIDs.length) {
    params.push('comment_ids=' + encodeURIComponent(replaceCommentIDs.join(',')));
  }
  var url = '/-/transactions/' + encodeURIComponent(txId) + '/timeline/rows';
  if (params.length) url += '?' + params.join('&');
  var anchorComposer = !!(opts && opts.anchorComposer);
  // Capture composer's pre-insertion viewport y-position when caller asked
  // to anchor it. We measure here (before the fetch) so the value is the
  // user's current view of the textarea — measuring after the fetch resolves
  // would already include any layout the network request triggered.
  var anchorEl = anchorComposer ? document.querySelector('[data-composer-anchor]') : null;
  var anchorBeforeY = anchorEl ? anchorEl.getBoundingClientRect().top : 0;
  return fetch(url, { headers: { Accept: 'text/html' } })
    .then(function (res) {
      if (!res.ok) throw new Error('render-failed');
      return res.text();
    })
    .then(function (html) {
      if (!html.trim()) return 0;
      // Parse the fragment. The server returns one or more <li> elements
      // (plus an optional day separator <li>) — no surrounding wrapper.
      var tpl = document.createElement('template');
      tpl.innerHTML = html.trim();
      var nodes = Array.prototype.slice.call(tpl.content.children);
      if (nodes.length === 0) return 0;

      // Insertion point: the timeline may already exist (timelineList())
      // or the page may have rendered txdTimelineEmptyComposer (no <ol>
      // yet). For the empty case we don't try to materialize the rail —
      // a single fresh row reading without it is acceptable; the next
      // page load will render the proper rail. The 99%-case is the
      // non-empty timeline.
      var ol = timelineList();
      var composer = composerLi();
      if (!ol) {
        // Empty-state fallback: full reload so the timeline gets its
        // proper <ol> rail. Rare path; acceptable.
        window.location.reload();
        return 0;
      }

      // Insert each node. For replacement rows (tombstones — the
      // returned <li> has data-comment-id matching one of the requested
      // replaceCommentIDs), find the matching bubble <li> and swap it
      // in place. For genuinely new rows, insert just before the
      // composer (so the composer stays at the bottom).
      var inserted = 0;
      var lastAppended = null;
      var replaceSet = {};
      if (replaceCommentIDs && replaceCommentIDs.length) {
        replaceCommentIDs.forEach(function (id) { replaceSet[id] = true; });
      }
      nodes.forEach(function (n) {
        var commentID = (n.dataset && n.dataset.commentId) || '';
        if (commentID && replaceSet[commentID]) {
          var existing = ol.querySelector('li[data-comment-id="' + commentID + '"]');
          if (existing) {
            existing.parentNode.replaceChild(n, existing);
            inserted++;
            return;
          }
        }
        if (composer) {
          ol.insertBefore(n, composer);
        } else {
          ol.appendChild(n);
        }
        inserted++;
        lastAppended = n;
      });

      // Post-insert scroll behavior. Two modes:
      //
      //   1. anchorComposer (comment add) — keep the composer pinned to
      //      the same viewport y-position it occupied before insertion.
      //      The user was just typing into the textarea; scrolling the
      //      new row into view would push the textarea off-screen and
      //      make them feel they "lost their place". The new comment row
      //      lands in its natural spot above the composer; if it ends up
      //      above the viewport, scrolling up reveals it.
      //
      //   2. default (category set, tag add, etc.) — bring the last
      //      appended row into view so the optimistic update is visible
      //      on small viewports. The user wasn't typing into anything,
      //      so there's no input focus to preserve, and the motion
      //      doubles as a "your action landed" signal.
      //
      // Both modes skip when there's nothing newly appended (tombstone-
      // only swaps): the bubble being swapped is already on screen if
      // the user just clicked its trash button.
      if (anchorComposer && anchorEl) {
        // rAF so layout settles after the new <li> is in the tree, then
        // restore the composer to its pre-insertion y-position via an
        // instant scrollBy (no smooth animation — the goal is invisible
        // pinning, not motion).
        window.requestAnimationFrame(function () {
          var afterY = anchorEl.getBoundingClientRect().top;
          var delta = afterY - anchorBeforeY;
          if (delta !== 0) {
            window.scrollBy({ top: delta, left: 0, behavior: 'instant' });
          }
        });
      } else if (lastAppended) {
        window.requestAnimationFrame(function () {
          lastAppended.scrollIntoView({ behavior: 'smooth', block: 'end' });
        });
      }

      // Update the cursor so the next fetch only sees newer rows. Every
      // returned <li> has a <time datetime="..."> child for the activity
      // entry's timestamp; the last one is the newest. Skip rows that
      // were replacements (older timestamps would regress the cursor).
      var lastTime = null;
      for (var i = nodes.length - 1; i >= 0; i--) {
        var nodeID = (nodes[i].dataset && nodes[i].dataset.commentId) || '';
        if (nodeID && replaceSet[nodeID]) continue;
        var t = nodes[i].querySelector('time[datetime]');
        if (t) { lastTime = t.getAttribute('datetime'); break; }
      }
      if (lastTime) root.dataset.lastActivityTs = lastTime;

      // Re-bootstrap Lucide icons inside the freshly-inserted rows.
      if (window.lucide && typeof window.lucide.createIcons === 'function') {
        window.lucide.createIcons();
      }
      // Render markdown inside any freshly-inserted comment bubbles.
      // The shared scanner is loaded as a sibling script in
      // transaction_detail.templ; it auto-runs on DOMContentLoaded for
      // server-rendered rows but optimistic inserts (and tombstone
      // swaps) bypass that, so we re-scan each new node here. Idempotent
      // — already-rendered bubbles carry data-markdown-rendered="1".
      if (typeof window.bbRenderMarkdown === 'function') {
        nodes.forEach(function (n) { window.bbRenderMarkdown(n); });
      }
      return inserted;
    });
}

// notifyNeedsReviewChanged dispatches the cross-factory sync event when a
// successful timeline-driven add/remove of the `needs-review` slug lands.
// txdCommentManager listens and updates its bound state so the composer's
// "Add Needs Review" affordance reflects the new truth without a reload.
// Caller is responsible for slug-scoping; this just fires the event.
function notifyNeedsReviewChanged(isOn) {
  window.dispatchEvent(new CustomEvent('bb-needs-review-changed', { detail: { isOn: !!isOn } }));
}

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
        var prevOverride = this.isOverride;
        this.saving = true;
        // Optimistic chip-state update: flip override badge immediately
        // so the sidebar reads as "manual" while the request flies.
        this.isOverride = true;
        fetch('/-/transactions/' + this.txId + '/category', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ category_id: detail.id }),
        })
          .then(function (r) {
            if (!r.ok && r.status !== 204) {
              return r.json().then(function (d) {
                throw new Error((d && d.error && d.error.message) || 'Failed to update category.');
              });
            }
            return fetchTimelineRows(self.txId);
          })
          .then(function () {
            self.saving = false;
            showToast('Category updated.', 'success');
          })
          .catch(function (e) {
            self.saving = false;
            self.isOverride = prevOverride;
            restorePageState();
            showToast((e && e.message) || 'Network error.');
          });
      },

      resetCategory: function () {
        var self = this;
        var prevOverride = this.isOverride;
        this.saving = true;
        this.isOverride = false;
        fetch('/-/transactions/' + this.txId + '/category', { method: 'DELETE' })
          .then(function (r) {
            if (!r.ok && r.status !== 204) {
              throw new Error('Failed to reset category.');
            }
            return fetchTimelineRows(self.txId);
          })
          .then(function () {
            self.saving = false;
            showToast('Category reset to auto-detected.', 'success');
          })
          .catch(function (e) {
            self.saving = false;
            self.isOverride = prevOverride;
            restorePageState();
            showToast((e && e.message) || 'Network error.');
          });
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

      // tagFromAvailable looks up the {displayName, color, icon} for a
      // slug from the seeded availableTags list. Falls back to a chip
      // displaying the slug verbatim when the tag isn't in the registry
      // (rare; e.g. created concurrently elsewhere).
      tagFromAvailable: function (slug) {
        for (var i = 0; i < this.availableTags.length; i++) {
          var t = this.availableTags[i];
          if (t.slug === slug) {
            return {
              slug: t.slug,
              displayName: t.display_name || t.slug,
              color: t.color,
              icon: t.icon,
            };
          }
        }
        return { slug: slug, displayName: slug, color: null, icon: null };
      },

      applyTagDiff: function (adds, removes) {
        adds = adds || [];
        removes = removes || [];
        if (adds.length + removes.length === 0) return;
        var self = this;
        var prevTags = this.tags.slice();
        // Optimistic chip-state update: add new chips, drop removed ones.
        var tagsCopy = prevTags.slice();
        removes.forEach(function (slug) {
          tagsCopy = tagsCopy.filter(function (t) { return t.slug !== slug; });
        });
        adds.forEach(function (slug) {
          if (!tagsCopy.some(function (t) { return t.slug === slug; })) {
            tagsCopy.push(self.tagFromAvailable(slug));
          }
        });
        this.tags = tagsCopy;
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
            if (!(res.ok && res.body && res.body.succeeded > 0)) {
              var msg = (res.body && res.body.error && res.body.error.message) || 'Failed to update tags.';
              throw new Error(msg);
            }
            return fetchTimelineRows(self.txId);
          })
          .then(function () {
            // Slug-scoped: only dispatch when the diff touched needs-review.
            // The composer toggle's hasPendingReview/pinReview state only
            // cares about that one slug.
            if (adds.indexOf('needs-review') !== -1) notifyNeedsReviewChanged(true);
            else if (removes.indexOf('needs-review') !== -1) notifyNeedsReviewChanged(false);
            showToast('Tags updated.', 'success');
          })
          .catch(function (e) {
            self.tags = prevTags;
            restorePageState();
            showToast((e && e.message) || 'Network error.');
          });
      },

      removeTag: function (tag, note) {
        var self = this;
        var prevTags = this.tags.slice();
        this.tags = prevTags.filter(function (t) { return t.slug !== tag.slug; });
        // Inline x on a tag chip - direct DELETE. Note is optional.
        var url = '/-/transactions/' + this.txId + '/tags/' + encodeURIComponent(tag.slug);
        fetch(url, {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ note: note || '' }),
        })
          .then(function (res) { return res.json().then(function (d) { return { ok: res.ok, body: d }; }); })
          .then(function (r) {
            if (!r.ok) {
              throw new Error((r.body && (r.body.error?.message || r.body.error)) || 'Failed to remove tag.');
            }
            return fetchTimelineRows(self.txId);
          })
          .then(function () {
            // Slug-scoped sync to the composer toggle (see header comment).
            if (tag.slug === 'needs-review') notifyNeedsReviewChanged(false);
            showToast('Tag removed.', 'success');
          })
          .catch(function (e) {
            self.tags = prevTags;
            restorePageState();
            showToast((e && e.message) || 'Network error.');
          });
      },
    };
  });

  // Activity-timeline comment composer. Reads txId + maxLength +
  // hasPendingReview from data-*. The `Add Needs Review` checkbox in the
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
      submitting: false,

      init: function () {
        var ds = this.$el.dataset;
        this.txId = ds.txId || '';
        var parsed = parseInt(ds.maxCommentLength, 10);
        if (!isNaN(parsed) && parsed > 0) this.maxLength = parsed;
        this.hasPendingReview = ds.hasPendingReview === 'true';

        // Cross-factory sync (Option B). When the timeline tag manager
        // adds or removes the needs-review slug, mirror the change on the
        // composer's bound state so the "Add Needs Review" affordance
        // hides/shows correctly without a page reload. Idempotent — bail
        // when the state already matches so the composer's own toggle
        // path doesn't fight itself.
        var self = this;
        window.addEventListener('bb-needs-review-changed', function (e) {
          var next = !!(e && e.detail && e.detail.isOn);
          if (self.hasPendingReview === next) return;
          self.hasPendingReview = next;
          if (next) self.pinReview = false;
        });
      },

      canSubmit: function () {
        var trimmed = this.newComment.trim().length;
        return !this.submitting && trimmed > 0 && this.newComment.length <= this.maxLength;
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
        var prevPinReview = this.pinReview;
        this.submitting = true;
        fetch('/-/transactions/' + this.txId + '/comments', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content: content }),
        })
          .then(function (res) {
            if (!res.ok) {
              return res.json().then(function (data) {
                throw new Error((data && data.error) || 'Failed to add comment.');
              });
            }
            // Optionally chain the pin tag write.
            if (!shouldPin) return null;
            return fetch('/-/transactions/' + self.txId + '/tags', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ slug: 'needs-review', note: '' }),
            }).then(function (tagRes) {
              if (!tagRes.ok) throw new Error('Tag write failed');
              return null;
            });
          })
          .then(function () {
            // Anchor the composer to its current viewport position so the
            // textarea the user was just typing into doesn't get pushed
            // off-screen by the optimistic comment row.
            return fetchTimelineRows(self.txId, null, { anchorComposer: true });
          })
          .then(function () {
            self.newComment = '';
            self.submitting = false;
            if (shouldPin) {
              // Update local state so subsequent posts don't try to pin again
              // and the composer's "Add Needs Review" checkbox hides.
              self.hasPendingReview = true;
              self.pinReview = false;
              showToast('Comment added; tagged needs-review.', 'success');
            } else {
              showToast('Comment added.', 'success');
            }
            // Reset textarea height after successful clear.
            var ta = document.getElementById('bb-txd-comment');
            if (ta) self.autosize(ta);
          })
          .catch(function (e) {
            self.submitting = false;
            self.pinReview = prevPinReview;
            restorePageState();
            // Comment write may have succeeded even if the pin tag write
            // failed; surface a "warning" tone in that path.
            var msg = (e && e.message) || 'Network error.';
            if (msg === 'Tag write failed') {
              showToast('Comment added (tag failed).', 'warning');
              // Refresh timeline so the comment row still appears even when
              // the secondary tag write was the one that failed. Same
              // composer-anchoring rationale as the success path.
              fetchTimelineRows(self.txId, null, { anchorComposer: true }).then(function () {
                self.newComment = '';
                var ta = document.getElementById('bb-txd-comment');
                if (ta) self.autosize(ta);
              }).catch(function () {});
            } else {
              showToast(msg);
            }
          });
      },

      deleteComment: function (id) {
        var self = this;
        bbConfirm({ title: 'Delete comment?', message: 'This comment will be permanently removed.', confirmLabel: 'Delete', variant: 'danger' }).then(function (ok) {
          if (!ok) return;
          fetch('/-/transactions/' + self.txId + '/comments/' + id, {
            method: 'DELETE',
          })
            .then(function (res) {
              if (!res.ok && res.status !== 204) {
                return res.json().then(function (data) {
                  throw new Error((data && data.error) || 'Failed to delete comment.');
                });
              }
              // Pass the deleted comment's short_id as a replacement
              // ID. The render endpoint will return the tombstone row
              // for that annotation (PR 4 soft-delete keeps the
              // annotation in place but flips is_deleted=true), and
              // fetchTimelineRows swaps it in over the original bubble
              // by matching data-comment-id.
              return fetchTimelineRows(self.txId, [id]);
            })
            .then(function () { showToast('Comment deleted.', 'success'); })
            .catch(function (e) {
              restorePageState();
              showToast((e && e.message) || 'Network error.');
            });
        });
      },
    };
  });
});
