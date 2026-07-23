<!-- project-memory:start -->
## Project memory
mode: loose
At session start, count files in `memories/` and report total and recent
counts; do not load every memory. Before any task, search memory frontmatter
for relevant terms and load only selected matches, ranking `CRITICAL` impact,
exact relevance, and recency first. After resolving a non-obvious issue,
persist it via the project-memory skill.
<!-- project-memory:end -->

## UI theming: every drawn cell must carry the theme background
When adding widgets to any screen or overlay, style them — do NOT use a bare
`widget.NewText(...)`, and always set a divider style on `widget.Split`
(`SetStyle(styles.Cell("panels.border"))`, not just `SetBorderChars`). The tui
runtime fills the theme background each frame, but `widget.Text.Draw` and the
`Split` divider *overwrite* their region with the widget's own style. An unset
(background-less) style therefore punches terminal-default holes through the
theme, which shows up as "inconsistent theming" on any terminal whose default
background differs from the active theme. Give plain login labels a themed
style via the `loginLabel` helper (`internal/ui/login.go`); use the matching
semantic cell elsewhere (`login.input`, `auth.status`, `messages.content`,
etc.). Containers (`widget.Node`/Column/Row) and `widget.Border` do not fill
interiors, so the child is responsible for its own background. See
`memories/login-surface-background-holes.md` and
`memories/overlay-border-theming-seam.md`; guard new surfaces the way
`TestLoginSurfaceFullyThemed` does.
