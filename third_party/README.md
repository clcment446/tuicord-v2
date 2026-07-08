# third_party

## arikawa (patched fork)

`arikawa/` is a verbatim copy of `github.com/diamondburned/arikawa/v3` at the
pinned version from `go.mod` (`v3.6.1-0.20260311205148-176ad9b9440f`), wired in
with a `replace` directive. The full delta against the pristine module lives in
`arikawa-select-valuelimits.patch`.

### Why

Upstream tags `ValueLimits [2]int` with `json:"-"` on every select-family
component and only writes `min_values`/`max_values` in `MarshalJSON`. There is
no `UnmarshalJSON` counterpart, so the limits of incoming components are
silently dropped and a client cannot distinguish a multi-select from a single
select. The patch adds `discord/component_valuelimits_patch.go` with the
missing unmarshal implementations (all seven types carrying `ValueLimits`) and
deletes a `go.work` that ships in the module zip but references example
directories the zip does not contain.

This is a temporary fix; drop the fork once the fix lands upstream.

### Regenerating after a version bump

1. `go mod edit -dropreplace github.com/diamondburned/arikawa/v3`
2. Bump the dependency, `go mod download`, note the new module cache path.
3. Copy the cache dir to `third_party/arikawa`, `chmod -R u+w` it, remove
   `go.work`/`go.work.sum`.
4. Apply the patch: `patch -p1 -d third_party/arikawa < third_party/arikawa-select-valuelimits.patch`
   (only the added-file hunk matters; the go.work hunks may already be moot).
5. Restore the replace directive, run `go build ./...` and
   `go test github.com/diamondburned/arikawa/v3/discord`.
6. Regenerate the patch: diff the pristine cache dir (as `a`) against
   `third_party/arikawa` (as `b`) with `diff -ruN`.

`internal/app` has a regression test that fails if the replace directive or the
patch is ever lost.
