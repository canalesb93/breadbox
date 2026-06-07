//go:build !headless && !lite

package markdown

import (
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
)

// ChromaCSS returns the class-based stylesheet for a chroma style (e.g.
// "github", "github-dark"). The rendered code blocks use class tokens
// (highlighting.WithFormatOptions(chromahtml.WithClasses(true))), so the
// matching CSS must live in static/css/input.css.
//
// This is a build-time helper to (re)generate that CSS — it is not called
// at request time. Regenerate via TestDumpChromaCSS (see markdown_test.go)
// or a throwaway `go run` and paste the output under the
// "chroma syntax highlighting" block in input.css.
func ChromaCSS(styleName string) (string, error) {
	style := styles.Get(styleName) // falls back to "fallback" if unknown
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	var sb strings.Builder
	if err := formatter.WriteCSS(&sb, style); err != nil {
		return "", err
	}
	return sb.String(), nil
}
