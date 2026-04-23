package pages

import (
	"encoding/json"
	"fmt"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// CategoryFormProps mirrors the data map the old category_form.html
// read: the edit/create mode flag, the category record (in edit mode),
// the full tree (for the parent picker in create mode), and the
// breadcrumb trail.
type CategoryFormProps struct {
	IsEdit      bool
	Category    *service.CategoryResponse
	Categories  []service.CategoryResponse
	Breadcrumbs []components.Breadcrumb
}

// categoriesJSON marshals the parent-picker dataset. The helper exists so
// the templ template can emit the JSON directly into an x-data attribute
// without having to reach for encoding/json in markup.
func categoriesJSON(cs []service.CategoryResponse) string {
	b, err := json.Marshal(cs)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// categoryColorOr returns the current color value or a sensible default
// for the form's live preview. Keeps the preview header on-brand when a
// category has no color set.
func categoryColorOr(c *string, fallback string) string {
	if c != nil && *c != "" {
		return *c
	}
	return fallback
}

// categoryIconOr returns the current icon name or empty string for the
// preview's icon slot.
func categoryIconOr(c *string) string {
	if c == nil {
		return ""
	}
	return *c
}

// categoryIDOr returns the ID or empty string — used to embed the current
// category's UUID into a JS literal.
func categoryIDOr(c *service.CategoryResponse) string {
	if c == nil {
		return ""
	}
	return c.ID
}

// categoryDisplayOr returns the display name or empty string for preview
// fallback text.
func categoryDisplayOr(c *service.CategoryResponse) string {
	if c == nil {
		return ""
	}
	return c.DisplayName
}

// categorySlugOr returns the slug or empty string.
func categorySlugOr(c *service.CategoryResponse) string {
	if c == nil {
		return ""
	}
	return c.Slug
}

// categorySortOrderOr returns the sort order or 0.
func categorySortOrderOr(c *service.CategoryResponse) int32 {
	if c == nil {
		return 0
	}
	return c.SortOrder
}

// categoryHiddenOr returns the hidden flag or false.
func categoryHiddenOr(c *service.CategoryResponse) bool {
	if c == nil {
		return false
	}
	return c.Hidden
}

// jsStringLit marshals a string to a JSON-safe JS string literal, including
// surrounding quotes. Used for interpolating Go values into a hand-written
// <script> body.
func jsStringLit(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// categoryIconPtr returns p.Category.Icon or nil when no category is set.
func categoryIconPtr(c *service.CategoryResponse) *string {
	if c == nil {
		return nil
	}
	return c.Icon
}

// categoryColorPtr returns p.Category.Color or nil when no category is set.
func categoryColorPtr(c *service.CategoryResponse) *string {
	if c == nil {
		return nil
	}
	return c.Color
}

// categoryFormBootstrap renders the Alpine bootstrap <script> for the
// category form. Extracting it as a function keeps the templ template
// clean and lets us interpolate Go values via jsStringLit rather than
// templating inside a <script> block.
func categoryFormBootstrap(p CategoryFormProps) string {
	return fmt.Sprintf(`<script>
function categoryForm() {
  var isEdit = %t;
  var categoryId = %s;
  var initial = {
    displayName: %s,
    parentId: '',
    icon: %s,
    color: %s,
    sortOrder: %d,
    hidden: %t
  };
  return {
    isEdit: isEdit,
    categoryId: categoryId,
    displayName: initial.displayName,
    parentId: initial.parentId,
    icon: initial.icon,
    color: initial.color,
    sortOrder: initial.sortOrder,
    hidden: initial.hidden,
    submitting: false,
    error: '',
    showIconPicker: false,
    commonIcons: ['shopping-cart','utensils','car','home','briefcase','heart','zap','book','plane','music','gift','coffee','scissors','shirt','gamepad-2','graduation-cap','building-2','fuel','bus','train'],
    presetColors: ['#6366f1','#8b5cf6','#ec4899','#ef4444','#f97316','#eab308','#22c55e','#14b8a6','#06b6d4','#3b82f6','#64748b','#78716c'],

    restorePageState: function() {
      if (window.bbProgress) window.bbProgress.finish();
      var main = document.querySelector('main');
      if (main) { main.style.opacity = ''; main.style.filter = ''; main.style.pointerEvents = ''; }
    },

    submit: function() {
      this.error = '';
      var displayName = (this.displayName || '').trim();
      if (!displayName) { this.error = 'Display name is required.'; return; }
      var body, url, method;
      if (this.isEdit) {
        url = '/-/categories/' + this.categoryId;
        method = 'PUT';
        body = {
          display_name: displayName,
          icon: this.icon || null,
          color: this.color || null,
          sort_order: this.sortOrder | 0,
          hidden: !!this.hidden
        };
      } else {
        url = '/-/categories';
        method = 'POST';
        body = {
          display_name: displayName,
          parent_id: this.parentId || null,
          icon: this.icon || null,
          color: this.color || null,
          sort_order: this.sortOrder | 0
        };
      }
      var self = this;
      self.submitting = true;
      fetch(url, { method: method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body) })
        .then(function(res) {
          if (res.ok) { window.location.href = '/categories'; return; }
          return res.json().then(function(data) {
            self.submitting = false;
            self.restorePageState();
            var msg = 'Failed to save category.';
            if (data && data.error) {
              if (typeof data.error === 'object' && data.error.message) msg = data.error.message;
              else if (typeof data.error === 'string') msg = data.error;
            }
            self.error = msg;
          }).catch(function() {
            self.submitting = false;
            self.restorePageState();
            self.error = 'Failed to save category.';
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
		jsStringLit(categoryIDOr(p.Category)),
		jsStringLit(categoryDisplayOr(p.Category)),
		jsStringLit(categoryIconOr(categoryIconPtr(p.Category))),
		jsStringLit(categoryColorOr(categoryColorPtr(p.Category), "#6366f1")),
		categorySortOrderOr(p.Category),
		categoryHiddenOr(p.Category),
	)
}
