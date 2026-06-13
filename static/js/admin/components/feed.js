// Feed page Alpine factories for /feed.
//
// Convention reference: docs/design-system.md → "Alpine page components".
//
//   - `feedSyncNow` — backs the "Sync now" button on the empty-state card.
//     Triggers a household-wide sync (POST /-/connections/sync-all) and
//     reloads the page once the kickoff returns 2xx so the freshly written
//     sync_logs surface in the feed timeline. Admin-only; the templ guards
//     the button with `if p.IsAdmin`, so this factory is only ever bound
//     under an admin session.
//
//   - `feedPagination` — backs the inline "Load older activity" pagination.
//     Instead of navigating to `/?before=…` (which reloads the page from the
//     top), it GETs the next window's rail rows from `/-/feed/rows` and
//     appends them into the existing <ol class="bb-timeline">, so the timeline
//     continues in place. State (cursor, dedup anchor, at-cap) seeds from the
//     wrapper's data-* attrs and advances from the response headers. The
//     plain <a href> is the no-JS fallback.

document.addEventListener('alpine:init', function () {
  Alpine.data('feedPagination', function () {
    return {
      before: '',
      filter: '',
      lastDay: '',
      atMax: false,
      loading: false,
      expanded: false,

      init: function () {
        var d = this.$root.dataset;
        this.before = d.feedBefore || '';
        this.filter = d.feedFilter || '';
        this.lastDay = d.feedLastDay || '';
        this.atMax = d.feedAtMax === '1';
      },

      loadOlder: function () {
        if (this.loading || this.atMax || !this.before) return;
        this.loading = true;
        var self = this;

        var params = new URLSearchParams();
        params.set('before', this.before);
        if (this.filter) params.set('filter', this.filter);
        if (this.lastDay) params.set('last_day', this.lastDay);

        fetch('/-/feed/rows?' + params.toString())
          .then(function (res) {
            if (!res.ok) throw new Error('feed rows: ' + res.status);
            var meta = {
              nextBefore: res.headers.get('X-Feed-Next-Before') || '',
              lastDay: res.headers.get('X-Feed-Last-Day') || '',
              atMax: res.headers.get('X-Feed-At-Max') === '1',
            };
            return res.text().then(function (html) {
              meta.html = html;
              return meta;
            });
          })
          .then(function (meta) {
            var ol = self.$root.querySelector('ol.bb-timeline');
            if (ol && meta.html.trim() !== '') {
              ol.insertAdjacentHTML('beforeend', meta.html);
              // Server rows ship as inline SVG, but render any JS-bound
              // <i data-lucide> placeholders the rows may carry.
              if (window.lucide && typeof window.lucide.createIcons === 'function') {
                window.lucide.createIcons();
              }
            }
            if (meta.nextBefore) self.before = meta.nextBefore;
            if (meta.lastDay) self.lastDay = meta.lastDay;
            self.expanded = true;
            self.loading = false;
            if (meta.atMax) {
              self.atMax = true;
              self.showEndOfFeed();
            }
          })
          .catch(function () {
            self.loading = false;
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Couldn’t load older activity. Please try again.', type: 'error' } }));
          });
      },

      // Replace the footer button with the terminal "End of feed" note once
      // the lookback cap is reached. Built in JS (not pre-rendered hidden) so
      // the in-window server render never contains the end-of-feed copy.
      showEndOfFeed: function () {
        var footer = this.$refs.paginator;
        if (footer) {
          footer.innerHTML = '<p class="text-xs text-base-content/40">End of feed</p>';
        }
      },
    };
  });

  // feedCountUp animates a hero metric number from 0 to its target on
  // load. The element ships with its final value as text content (no-JS
  // fallback) plus a `data-count-target` attr; this tween overwrites it
  // for the ~0.7s reveal, then snaps to the exact target. Honors
  // prefers-reduced-motion by skipping straight to the final value.
  Alpine.data('feedCountUp', function () {
    return {
      init: function () {
        var el = this.$el;
        var target = parseInt(el.getAttribute('data-count-target') || '0', 10);
        if (!target || target < 0) return;
        var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        if (reduce) { el.textContent = String(target); return; }
        var dur = 700, start = null;
        el.textContent = '0';
        function step(ts) {
          if (start === null) start = ts;
          var p = Math.min((ts - start) / dur, 1);
          var eased = 1 - Math.pow(1 - p, 3);
          el.textContent = String(Math.round(eased * target));
          if (p < 1) {
            requestAnimationFrame(step);
          } else {
            el.textContent = String(target);
          }
        }
        requestAnimationFrame(step);
      },
    };
  });

  Alpine.data('feedSyncNow', function () {
    return {
      state: 'idle',

      triggerSyncNow: function () {
        if (this.state !== 'idle') return;
        this.state = 'syncing';
        var self = this;
        fetch('/-/connections/sync-all', { method: 'POST' })
          .then(function (res) {
            if (res.ok) {
              self.state = 'done';
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Sync triggered. Reloading…', type: 'success' } }));
              setTimeout(function () { window.location.reload(); }, 1200);
            } else {
              self.state = 'idle';
              window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Failed to trigger sync.', type: 'error' } }));
            }
          })
          .catch(function () {
            self.state = 'idle';
            window.dispatchEvent(new CustomEvent('bb-toast', { detail: { message: 'Network error. Please try again.', type: 'error' } }));
          });
      },
    };
  });
});
