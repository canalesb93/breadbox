package admin

import (
	"bytes"
	"context"
	"html/template"
	"reflect"

	"breadbox/internal/templates/components"

	"github.com/a-h/templ"
)

// renderComponent is the generic bridge that lets html/template call any templ
// component. It's registered in the funcMap as "renderComponent" and used by
// component-specific helpers (tagChip, tagChipSm, ...) that adapt data.
//
// Returning template.HTML tells html/template the output is already escaped,
// which is safe because templ performs its own context-aware escaping.
func renderComponent(c templ.Component) template.HTML {
	if c == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return template.HTML("<!-- templ render error: " + template.HTMLEscapeString(err.Error()) + " -->")
	}
	return template.HTML(buf.String())
}

// tagChipFromAny adapts an arbitrary struct (service.TagResponse,
// service.AdminTransactionTag, or any page-local type with matching fields) to
// the flat TagChipData the templ component consumes. It mirrors how the legacy
// html/template partial resolved `.Slug`, `.DisplayName`, `.Color`, `.Icon`
// via reflection, so callers keep passing domain types unchanged.
func tagChipFromAny(v any) components.TagChipData {
	var d components.TagChipData
	if v == nil {
		return d
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return d
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return d
	}
	if f := rv.FieldByName("Slug"); f.IsValid() && f.Kind() == reflect.String {
		d.Slug = f.String()
	}
	if f := rv.FieldByName("DisplayName"); f.IsValid() && f.Kind() == reflect.String {
		d.DisplayName = f.String()
	}
	d.Color = stringPtrField(rv, "Color")
	d.Icon = stringPtrField(rv, "Icon")
	return d
}

// stringPtrField extracts a string-valued field by name as *string, accepting
// either a string or *string on the source struct. Returns nil when the field
// is missing or a nil pointer; the templ component treats empty strings the
// same as nil.
func stringPtrField(rv reflect.Value, name string) *string {
	f := rv.FieldByName(name)
	if !f.IsValid() {
		return nil
	}
	switch f.Kind() {
	case reflect.Pointer:
		if f.IsNil() || f.Elem().Kind() != reflect.String {
			return nil
		}
		s := f.Elem().String()
		return &s
	case reflect.String:
		s := f.String()
		return &s
	}
	return nil
}

// tagChipFunc renders the default-size tag chip component from any struct
// exposing the canonical fields (Slug/DisplayName/Color/Icon).
func tagChipFunc(v any) template.HTML {
	return renderComponent(components.TagChip(tagChipFromAny(v)))
}

// tagChipSmFunc renders the compact tag chip used in dense list rows.
func tagChipSmFunc(v any) template.HTML {
	return renderComponent(components.TagChipSm(tagChipFromAny(v)))
}
