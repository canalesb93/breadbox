# Markdown rendering

One server-side renderer ([goldmark](https://github.com/yuin/goldmark) →
[bluemonday](https://github.com/microcosm-cc/bluemonday)) produces the HTML for
every Markdown surface: agent transcripts, reports, transaction comments, and
workflow prompt previews. No client-side parser. Source: `internal/markdown/`;
output is wrapped in `.bb-prose` by the caller.

Standard **CommonMark + GFM** is supported as you'd expect. This doc only covers
the non-obvious bits.

## Callouts (GitHub-style admonitions)

A blockquote whose **first line, alone,** is `[!TYPE]` renders as a colored,
icon'd box:

```markdown
> [!NOTE]
> Body goes here. Markdown inside works.
```

Five types: `NOTE` · `TIP` · `IMPORTANT` · `WARNING` · `CAUTION`. The marker
must be on its own line — trailing text on the marker line (`> [!NOTE] hi`)
falls back to a plain blockquote so nothing is dropped.

## Tables

GFM tables, with per-column alignment via the `:` in the delimiter row:

```markdown
| Left | Center | Right |
|:-----|:------:|------:|
| a    | b      | c     |
```

## Task lists

```markdown
- [x] done
- [ ] todo
```

Checkboxes are **read-only** (rendered `disabled`) — display only, not
interactive.

## Fenced code blocks

Syntax-highlighted server-side ([chroma](https://github.com/alecthomas/chroma),
class-based — themed by CSS for light/dark, no JS). Each block gets chrome: a
language label pill and a copy button.

````markdown
```sql
SELECT * FROM transactions;
```
````

The language tag drives highlighting; `text`/`plaintext`/no-tag render plain
(no language pill).

## Headings

Every heading gets an auto-generated `id` (slug of its text) and a `#`
hover-anchor for deep-linking. Heading levels are **not** shifted — `#` stays
`<h1>`; `.bb-prose` CSS keeps them visually modest.

## Also on

- **Strikethrough** — `~~text~~`
- **Autolinks** — bare URLs become links
- **Typographer** — straight quotes → curly, `--`/`---` → en/em dash, `...` → ellipsis
- External links open in a new tab with `rel="noreferrer"`

## Not supported / deliberately off

- **Raw HTML is dropped.** Any `<tag>` in the source is stripped by goldmark
  (unsafe mode off) and the output is sanitized as a second pass — untrusted
  agent/user content can't inject markup. Allowed elements/attributes are
  whitelisted in `sanitize.go`.
- Footnotes, definition lists, Mermaid, math — not enabled (one extension each
  to add later if a real need shows up).

## Hard wraps

Transaction comments render in hard-wrap mode (a single newline → `<br>`, like
chat). Everywhere else uses standard CommonMark paragraph folding. Callers opt
in with `markdown.WithHardWraps()`.
