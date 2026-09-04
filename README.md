# sheen

Render HTML in the terminal, with pizzazz! The Glow experience, for HTML
instead of Markdown.

## Install

One-liner (macOS and Linux):

```
curl -fsSL https://raw.githubusercontent.com/hangarbay/sheen/master/install.sh | sh
```

Or download a tarball from the
[releases page](https://github.com/hangarbay/sheen/releases) (checksums
included), or use Go:

```
go install github.com/hangarbay/sheen/cmd/sheen@latest
```

## Usage

```
sheen [SOURCE|DIR]
```

`SOURCE` can be:

- a local HTML file: `sheen page.html`
- a URL: `sheen https://example.com` (scheme optional: `sheen example.com`)
- `-` or a pipe: `curl -s https://example.com | sheen`
- a directory: `sheen docs/` renders `docs/index.html`

## Flags

| Flag | Description |
|------|-------------|
| `-s, --style` | `auto`, `dark`, `light`, `notty`, `ascii` |
| `-w, --width` | word-wrap width (default: terminal width) |
| `-p, --pager` | page through `$PAGER` (default `less -r`) |
| `-t, --tui` | interactive scrollable TUI (`q` quit, vim/arrow keys, `g`/`G` top/bottom, `d`/`u` half-page) |
| `--links` | `auto`, `osc8` (clickable hyperlinks), `inline`, `none` |
| `--version` | print version |

## What it renders

- Headings, paragraphs, lists (nested, ordered with `start`), tables,
  blockquotes, horizontal rules, definition lists
- Links as OSC 8 clickable hyperlinks (or inline URLs)
- `<pre>` code blocks with Chroma syntax highlighting when a `language-*`
  class is present
- A practical CSS subset: tag/class/id selectors with descendant and child
  combinators, specificity-based cascade, `style` attributes, colors,
  bold/italic/underline, `display:none`, `text-align`, `white-space:pre`
- `<title>` as a fallback heading when the page has no `<h1>`
- Relative URLs resolved against the page's URL; remote pages may load
  their `<link rel="stylesheet">` stylesheets (http/https only)

## What it never does

- Executes JavaScript. `<script>` content is skipped entirely; `<noscript>`
  content is rendered, like a no-JS browser.
