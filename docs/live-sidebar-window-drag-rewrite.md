# Live window-to-sidebar drag rewrite

## Goal

Replace the previous live-drag preview approach with a layout-first implementation. A dragged window must become a real member of the sidebar card layout as soon as the pointer enters the strip; it must not wait for the shrink animation to finish.

## Required interaction contract

1. **Enter:** detect the dragged window crossing into the active strip. Create a transient card/slot in the display model immediately and run layout. The slot participates in the same layout pass as every other card, so neighboring cards begin moving aside on the first animation frame.
2. **Shrink:** render the dragged window from a captured still on the existing flight layer. Its destination is recomputed from the transient card's current layout pose, not from a guessed/static coordinate. The real window and the flight must never both be visibly presented.
3. **Track:** while the pointer remains in the strip, keep the transient card in the normal card ordering and keep the flight destination synchronized with scrolling, hover lift/tilt, and layout changes.
4. **Exit:** remove the transient slot immediately, relayout the remaining cards, and reverse from the flight's current interpolated pose toward the live window bounds. Do not restart from either endpoint or wait for the shrink to finish.
5. **Release:** commit the final card/window ownership exactly once. Pointer-up must not start a second animation or replay a completed preview.
6. **Cancel/lifecycle:** window destruction, drag cancellation, display removal, and settings changes must clear transient state and release captured resources safely.

## Architecture constraints

- Keep this preview separate from `switchStage` and the stage dwell behavior in `dragOver`; a pointer crossing the strip is not a stage switch.
- Model the transient slot explicitly in the display/card layout. Do not fake participation by drawing a flight over the old layout.
- Reuse the existing flight rendering primitives where practical, but give live drag previews their own state and frame progression so switch trips and drag previews cannot overwrite each other.
- Keep `drag.from` as the original window rectangle. Preserve the existing `dropHomes` behavior for restoring a parked window.
- All state mutation and animation advancement must remain on MyGo's main thread.

## Acceptance tests

- Entering the strip inserts the placeholder before the first shrink frame and causes neighboring card targets to change.
- Leaving during shrink reverses continuously from the exact current pose (no position, size, or tilt jump).
- Re-entering during expansion reverses continuously again.
- Pointer-up inside commits once; pointer-up outside restores the window once.
- Existing cross-display drag dwell behavior and ordinary card drag behavior remain unchanged.
- `go test ./...` and `go build ./...` pass; verify animation layering and DPI behavior on Windows.

## Implementation status

This document is the rewrite contract. The implementation and automated tests are still pending; this PR must remain a draft until those changes and validation are complete.
