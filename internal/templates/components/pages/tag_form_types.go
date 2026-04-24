package pages

import (
	"fmt"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// TagFormProps mirrors the data map the old tag_form.html read: the
// edit/create mode flag, the tag record (in edit mode), and the
// breadcrumb trail.
type TagFormProps struct {
	IsEdit      bool
	Tag         *service.TagResponse
	Breadcrumbs []components.Breadcrumb
}

// tagIDOr returns the tag ID or empty string when no tag is set.
func tagIDOr(t *service.TagResponse) string {
	if t == nil {
		return ""
	}
	return t.ID
}

// tagSlugOr returns the tag slug or empty string.
func tagSlugOr(t *service.TagResponse) string {
	if t == nil {
		return ""
	}
	return t.Slug
}

// tagDisplayOr returns the tag display name or empty string.
func tagDisplayOr(t *service.TagResponse) string {
	if t == nil {
		return ""
	}
	return t.DisplayName
}

// tagDescriptionOr returns the tag description or empty string.
func tagDescriptionOr(t *service.TagResponse) string {
	if t == nil {
		return ""
	}
	return t.Description
}

// tagIconOr returns the tag's icon name or empty string.
func tagIconOr(t *service.TagResponse) string {
	if t == nil || t.Icon == nil {
		return ""
	}
	return *t.Icon
}

// tagColorOr returns the tag's color or the default primary swatch.
func tagColorOr(t *service.TagResponse) string {
	if t == nil || t.Color == nil || *t.Color == "" {
		return "#6366f1"
	}
	return *t.Color
}

// tagFormBootstrap renders the Alpine component bootstrap. Extracting it
// as a Go function keeps the templ template clean and lets us interpolate
// Go values via jsStringLit rather than inside a <script> block.
func tagFormBootstrap(p TagFormProps) string {
	return fmt.Sprintf(`<script>
function tagForm() {
  var isEdit = %t;
  var tagId = %s;
  var initial = {
    slug: %s,
    displayName: %s,
    description: %s,
    color: %s,
    icon: %s
  };
  return {
    isEdit: isEdit,
    tagId: tagId,
    slug: initial.slug,
    displayName: initial.displayName,
    description: initial.description,
    color: initial.color,
    icon: initial.icon,
    submitting: false,
    error: '',
    presetColors: ['#6366f1','#8b5cf6','#ec4899','#ef4444','#f97316','#eab308','#22c55e','#14b8a6','#06b6d4','#3b82f6','#64748b','#78716c'],

    restorePageState: function() {
      if (window.bbProgress) window.bbProgress.finish();
      var main = document.querySelector('main');
      if (main) { main.style.opacity = ''; main.style.filter = ''; main.style.pointerEvents = ''; }
    },

    submit: function() {
      this.error = '';
      var displayName = (this.displayName || '').trim();
      var slug = (this.slug || '').trim();
      if (!this.isEdit && !slug) { this.error = 'Slug is required.'; return; }
      if (!displayName) { this.error = 'Display name is required.'; return; }
      var body, url, method;
      if (this.isEdit) {
        url = '/-/tags/' + this.tagId;
        method = 'PUT';
        body = {
          display_name: displayName,
          description: this.description || '',
          color: this.color || null,
          icon: this.icon || null
        };
      } else {
        url = '/-/tags';
        method = 'POST';
        body = {
          slug: slug,
          display_name: displayName,
          description: this.description || '',
          color: this.color || null,
          icon: this.icon || null
        };
      }
      var self = this;
      self.submitting = true;
      fetch(url, { method: method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body) })
        .then(function(res) {
          if (res.ok) { window.location.href = '/tags'; return; }
          return res.json().then(function(data) {
            self.submitting = false;
            self.restorePageState();
            var msg = 'Failed to save tag.';
            if (data && data.error) {
              if (typeof data.error === 'object' && data.error.message) msg = data.error.message;
              else if (typeof data.error === 'string') msg = data.error;
            }
            self.error = msg;
          }).catch(function() {
            self.submitting = false;
            self.restorePageState();
            self.error = 'Failed to save tag.';
          });
        })
        .catch(function() {
          self.submitting = false;
          self.restorePageState();
          self.error = 'Network error. Please try again.';
        });
    }
  };
}
</script>`,
		p.IsEdit,
		jsStringLit(tagIDOr(p.Tag)),
		jsStringLit(tagSlugOr(p.Tag)),
		jsStringLit(tagDisplayOr(p.Tag)),
		jsStringLit(tagDescriptionOr(p.Tag)),
		jsStringLit(tagColorOr(p.Tag)),
		jsStringLit(tagIconOr(p.Tag)),
	)
}
