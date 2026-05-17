
- **Iter 101 — DetailDialogHeader primitive (4 household-section dialogs)** ([#1217](https://github.com/canalesb93/breadbox/pull/1217))
  - Closed the gap iter 100 explicitly flagged. The four form-bearing
    `<Dialog>` consumers in `household-section` (AddMember,
    CreateLogin, ShareLink inline, ShareLink standalone) all
    hand-rolled `<DialogHeader>` + `<DialogTitle>` +
    `<DialogDescription>` with no icon affordance — sibling sheets
    have ridden `<DetailSheetHeader>` (iter 41) since they grew a
    second consumer. Four sites is above the "third consumer"
    promotion gate, so promoted to a primitive.
  - New `<DetailDialogHeader>` at
    `web/src/components/detail-dialog-header.tsx` — sibling of
    `<DetailSheetHeader>`. Same icon-tile (`bg-muted
    text-muted-foreground rounded-lg border size-9`) + optional
    eyebrow + title + description + optional trailing slot lockup,
    just routed through `<DialogTitle>` / `<DialogDescription>` so
    it composes with shadcn `<Dialog>` instead of `<Sheet>`. 27th
    shared primitive in the v2 vocabulary.
  - Visual win: each form-bearing dialog now leads with a
    rounded-lg icon tile (UserPlus for member-add flows, Link2 for
    share-link flows) — matches the iter 41 sheet header vocabulary
    exactly so the system reads as a coherent family across both
    surface modes (slide-in Sheet vs centered Dialog) instead of
    sheets feeling "designed" and dialogs feeling "stock shadcn".
  - Sandbox specimen at `/v2/sandbox?section=components` right
    after `<DetailSheetHeader>` so the sibling pair reads
    top-to-bottom (default density + with-eyebrow-and-trailing
    presets, matching the live consumers).
  - No `density` variant yet — the four live consumers all want
    the default rhythm. If a future surface needs the heavier
    `accent` rhythm (the iter 41 sheet header's `size-10 + p-6 +
    text-lg` shape), add a `density` prop here mirroring that API
    rather than re-opening `<DialogHeader>`. The two primitives
    should keep their props in lockstep.
  - Remaining centered `<Dialog>` consumers in `web/src/`:
    `settings-shell` (sr-only chrome on the settings modal itself —
    intentionally not a candidate; the modal *is* the chrome),
    `confirm-dialog` (composes `AlertDialog` for yes/no
    confirmations). The form-bearing gap iter 100 left open is now
    fully closed.
