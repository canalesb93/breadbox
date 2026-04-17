## Summary

Brief description of the changes.

## Changes

- ...

## Testing

- [ ] Unit tests pass (`go test ./...`)
- [ ] Integration tests pass (`make test-integration`) — if applicable
- [ ] Build succeeds (`go build ./cmd/breadbox`)

## UI evidence

Required for any admin UI / template / CSS / Alpine change. Capture with the `validate-ui` skill (Chrome DevTools MCP → img402), embed with inline HTML so the rendered size is bounded. Delete this section for non-UI PRs.

Single capture:

```html
<img src="https://i.img402.dev/<id>.jpg" width="800" alt="<page> — after">
```

Before/after (preferred for visual diffs):

```html
<table>
  <tr><th>Before</th><th>After</th></tr>
  <tr>
    <td><img src="https://i.img402.dev/<before>.jpg" width="400" alt="before"></td>
    <td><img src="https://i.img402.dev/<after>.jpg" width="400" alt="after"></td>
  </tr>
</table>
```

Responsive changes: include both desktop (1280) and mobile (390). Wrap any `fullPage: true` capture in `<details>` to keep the PR body readable.

## Related issues

Closes #
