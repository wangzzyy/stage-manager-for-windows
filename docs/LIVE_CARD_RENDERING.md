# Live sidebar card rendering: implementation plan

## Goal

Keep parked-window cards live at a high update rate without losing the existing card perspective, tilt, hover lift, or flight animations, while keeping the steady-state cost low.

This is an architecture/feasibility proposal. It deliberately does not replace the current card renderer until a GPU-resident capture texture can be consumed by the renderer without a per-frame CPU readback.

## What the current code does

- `internal/parking/parking.go` parks a window by moving its HWND outside the virtual desktop with `SetWindowPos`. It does not minimize or hide the window. The HWND and its normal window state remain available.
- `internal/winapi/thumbnail.go` already wraps `DwmRegisterThumbnail`, `DwmUpdateThumbnailProperties`, and `DwmUnregisterThumbnail`.
- `internal/app/lineup.go` uses DWM thumbnails for the current-stage lineup. DWM owns composition and updates the thumbnail as the source changes.
- Sidebar cards use `internal/app/media.go`: `winapi.Capture` calls `PrintWindow`, produces CPU-side `image.RGBA` frames, and stores them as `ui.Bitmap`. Captures are asynchronous and limited by a semaphore of three, but a still is refreshed only when the app requests another capture.
- `internal/app/stripview.go` applies `Scale`, `RotateY`, and `Perspective` to the card's window layer. The flight layer in `internal/app/flights.go` also draws cached `ui.Bitmap` images and applies per-frame poses.

## Why a DWM-thumbnail replacement is not sufficient

A DWM thumbnail can be positioned, cropped, shown/hidden, and given an opacity. Its destination is an axis-aligned rectangle. It is not a texture exposed to the app's renderer and cannot inherit the card's `RotateY` or perspective transform.

Putting a DWM thumbnail over the sidebar card would therefore make the live image disagree with the transformed card underneath it. It would also complicate clipping, rounded corners, card stacking, and the existing flight hand-off. DWM thumbnails remain a good fit for the untransformed stage lineup, where the project already uses them.

## Recommended target architecture

Use **Windows Graphics Capture (WGC) + a GPU-resident texture + the existing card/flight transforms**.

1. **Capture source:** create a WGC capture session for each eligible parked top-level HWND. The window remains non-minimized while parked, so it is a candidate source; actual frame delivery while off the virtual desktop must be verified on Windows.
2. **Event-driven frames:** consume frames when WGC reports new content. Do not poll with `PrintWindow` and do not capture every app on every UI frame.
3. **GPU residency:** keep the newest frame in a reusable D3D11 texture. Scale/crop to the card's required resolution on the GPU where possible. Avoid mapping the full-resolution frame to CPU memory on every update.
4. **Renderer integration:** add a native-texture image path to the MyGo renderer (or a narrowly scoped native image node) so the card can draw the WGC texture directly. The existing `ui.Bitmap.Update(image.RGBA)` path is CPU-fed; routing every WGC frame through it would add GPU readback and upload costs and defeats the main performance goal.
5. **Reuse existing transforms:** feed the live texture into the same window layer that currently draws `st.card`. Keep the current `Scale`, `RotateY`, `Perspective`, opacity, and layout code. The flight layer should be able to reference the same texture.
6. **Bound resource use:** start capture only for windows represented by visible cards (and at most the windows actually drawn in each stack); stop or suspend sessions when the strip is hidden, the card leaves the visible range, the HWND is destroyed, or the app shuts down. Reuse textures and cap their dimensions to the rendered card size plus a quality margin.
7. **Graceful fallback:** if WGC cannot create a session, stops delivering frames, or the renderer cannot import its texture, keep the current cached still/icon. Never block the UI thread waiting for a capture frame.

## Animation and hand-off rules

- At animation start, use the most recently completed WGC frame as the flight's initial image. Do not call `PrintWindow` synchronously from the animation path.
- Keep the same texture resource alive for the duration of a flight; do not recreate or resize it per frame.
- Continue updating the texture only if the capture session delivers a new frame and the flight policy allows live content. The pose itself remains driven by the existing animation clock.
- At landing, keep the live texture in the card layer. Do not briefly show a stale `ui.Bitmap` over it or register a DWM thumbnail on top of the perspective-transformed card.
- Retain the current DWM thumbnail path for the stage lineup; it has different geometry and already has an efficient compositor-owned implementation.

## Feasibility gates before enabling by default

1. **Capture while parked:** verify WGC produces frames for a normal HWND after the parking service moves it off the virtual desktop. Test GDI, Chromium/Electron, and GPU-rendered content. Some apps may pause rendering when off-screen; no capture API can force the source app to produce new frames.
2. **Native texture import:** inspect the exact MyGo rendering backend and prove a D3D11 texture can be sampled without a CPU readback. If that is not possible, compare the measured cost of a downscaled, dirty-frame CPU path before adopting it.
3. **Color and alpha:** verify BGRA/RGBA format, premultiplied alpha, color space, DPI, crop bounds, and device-loss/recreation behavior.
4. **Lifecycle:** test HWND destruction/reuse, display changes, DPI changes, app shutdown, capture access denial, and sessions that stop producing frames.
5. **Performance:** measure GPU time, CPU time, memory, and frame pacing with 1, 5, and 10 visible cards at 60/120/144 Hz. Compare idle and animated states against the current still-image implementation.

## Non-goals for the first implementation

- Do not replace the existing flight animation system.
- Do not put DWM thumbnails over perspective-transformed sidebar cards.
- Do not continuously call `PrintWindow` at 60 Hz.
- Do not start capture sessions for every parked window regardless of visibility.
- Do not claim that every application will keep rendering while parked.

## Suggested implementation sequence

1. Add a Windows-only WGC capture prototype for one HWND and log frame arrival while the HWND is parked.
2. Identify the MyGo renderer's texture-import boundary and implement a minimal native D3D texture image.
3. Add a per-visible-card capture manager with bounded sessions and deterministic cleanup.
4. Wire the live image into the existing card window layer, leaving all current transforms intact.
5. Reuse the same texture in flights and validate frame-perfect hand-off.
6. Enable progressively after the capture, lifecycle, and performance tests pass.
