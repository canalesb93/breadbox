package admin

import (
	"bytes"
	"context"
	"html/template"

	"breadbox/internal/templates/components"
)

// renderFlashComponent bridges the templ FlashBanner component into
// html/template layouts via the funcMap. Accepts `any` because `.Flash`
// arrives as `interface{}` when page data is a `map[string]any`, and
// html/template won't coerce it back to a concrete pointer type.
func renderFlashComponent(v any) (template.HTML, error) {
	var f *Flash
	switch x := v.(type) {
	case *Flash:
		f = x
	case Flash:
		f = &x
	}
	if f == nil {
		return "", nil
	}
	var buf bytes.Buffer
	if err := components.FlashBanner(&components.Flash{
		Type:    f.Type,
		Message: f.Message,
	}).Render(context.Background(), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}
