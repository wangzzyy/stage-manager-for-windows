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

## Important renderer discovery

MyGo already has most of the *renderer-side* primitive needed for a zero-readback path:

- `internal/scene.Surface` can represent a native Windows shared Direct3D 11 BGRA texture using an NT handle (`Native`) and a stable `NativeID`.
- The D3D11 renderer opens that shared texture once, creates a shader-resource view, and synchronizes access with `IDXGIKeyedMutex`.
- MyGo's Windows Media Foundation video engine already demonstrates the producer side: it keeps a ring of three shared textures, writes each frame on the GPU, and publishes them as `scene.Surface` objects.
- This means MyGo's renderer already knows how to sample native shared D3D11 textures as normal images; a separate renderer or a CPU readback loop should not be necessary.

The remaining integration boundary is API visibility: `scene.Surface` is in MyGo's `internal` package, and `ui.Bitmap.showPicture` is private. Stage Manager cannot construct and attach a native surface through the public `ui.Bitmap` API today. The clean solution is a small public MyGo producer API (or an upstream change) that safely wraps a shared BGRA D3D11 surface and its lifetime. Avoid copying MyGo's renderer internals into Stage Manager.

### Proposed MyGo API contract

The app should not receive or mutate MyGo's internal `scene.Image`. A narrow public API should accept a native BGRA frame with:

- width and height;
- an owned shared NT handle and a stable texture ID;
- a producer synchronization contract (keyed mutex, key 0);
- optional CPU fallback pixels for renderers that cannot sample native surfaces;
- explicit ownership/lifetime semantics: the producer transfers or retains the handle until the bitmap no longer references that surface, and MyGo closes it only after the surface becomes unreachable.

The API should update an existing `ui.Bitmap` in place and mark its image changed, just as the internal video path does. It should not allocate a new bitmap/texture for every frame. This is the only renderer-facing API Stage Manager should need.

## Recommended target architecture

Use **Windows Graphics Capture (WGC) + a GPU-resident shared texture + the existing card/flight transforms**.

1. **Capture source:** create a WGC capture item for each eligible parked top-level HWND using `IGraphicsCaptureItemInterop.CreateForWindow`. The window remains non-minimized while parked, so it is a candidate source; actual frame delivery while off the virtual desktop must be verified on Windows.
2. **Capture device:** create a D3D11 device and expose it as a WinRT `IDirect3DDevice` with `CreateDirect3D11DeviceFromDXGIDevice`. Create a free-threaded WGC frame pool with BGRA8 frames and a small buffer count (start with three), then a capture session for the item.
3. **Capture frame:** consume frames only from WGC's `FrameArrived` event. Read the frame's content size and texture, then copy it on the GPU into one of three reusable shared BGRA textures. Do not map the full-resolution WGC frame to CPU memory.
4. **Shared output:** create the output textures with shared-NT-handle and keyed-mutex flags, following MyGo's existing Media Foundation producer pattern. Publish the selected ring slot's handle and stable ID through the proposed MyGo API. Use non-blocking/short-timeout mutex acquisition; if all slots are busy, drop that frame rather than stalling capture or the UI.
5. **Event-driven redraw:** coalesce frame-arrival notifications and request at most one UI redraw per display frame. The source controls the actual update rate; do not run a 144 Hz polling loop for a 30 Hz source.
6. **Reuse existing transforms:** feed the live native-surface image into the same window layer that currently draws `st.card`. Keep the current `Scale`, `RotateY`, `Perspective`, opacity, clipping, and layout code. The flight layer should be able to reference the same live image.
7. **Bound resource use:** start capture only for windows actually drawn in visible cards (including the visible stacked windows), not every window in every parked stage. Stop sessions when the strip is hidden, the card leaves the visible range, the HWND is destroyed, or the app shuts down. Reuse textures and cap their dimensions to the rendered card size plus a quality margin.
8. **Graceful fallback:** if WGC cannot create a session, stops delivering frames, or the renderer cannot import its texture, keep the current cached still/icon. Never block the UI thread waiting for a capture frame.

## Animation and hand-off rules

- At animation start, use the most recently completed WGC frame as the flight's initial image. Do not call `PrintWindow` synchronously from the animation path.
- Keep the same shared texture ring alive for the duration of a flight; do not recreate or resize it per frame.
- Continue updating the texture only when WGC delivers a new frame. The pose itself remains driven by the existing animation clock.
- At landing, keep the live image in the card layer. Do not briefly show a stale `ui.Bitmap` over it or register a DWM thumbnail on top of the perspective-transformed card.
- If a capture session is stopped because the strip is hidden, preserve the last completed frame as the still fallback so the card never goes blank when it reappears.
- Retain the current DWM thumbnail path for the stage lineup; it has different geometry and already has an efficient compositor-owned implementation.

## Feasibility gates before enabling by default

1. **Capture while parked:** verify WGC produces frames for a normal HWND after the parking service moves it off the virtual desktop. Test a GDI app with a changing clock, Chromium/Electron video, and GPU-rendered content. Record frame timestamps and compare them with the same window while on-stage. Some apps may pause their own rendering while off-screen; no capture API can force the source app to produce new frames.
2. **WGC-to-shared-texture path:** verify WGC frame textures can be copied into the shared ring on the GPU, keyed-mutex hand-off does not stall either device, and device recreation is handled.
3. **Public MyGo surface API:** design a lifetime-safe API that keeps each NT handle valid while the bitmap references it, uses a stable ID per ring slot, and releases producer resources only after the renderer no longer references them.
4. **Color and alpha:** verify BGRA format, premultiplied alpha, color space, DPI, crop bounds, and device-loss/recreation behavior.
5. **Lifecycle:** test HWND destruction/reuse, display changes, DPI changes, app shutdown, capture access denial, and sessions that stop producing frames.
6. **Performance:** measure GPU time, CPU time, memory, dropped frames, and frame pacing with 1, 5, and 10 visible cards at 60/120/144 Hz. Compare idle and animated states against the current still-image implementation.

## Non-goals for the first implementation

- Do not replace the existing flight animation system.
- Do not put DWM thumbnails over perspective-transformed sidebar cards.
- Do not continuously call `PrintWindow` at 60 Hz.
- Do not start capture sessions for every parked window regardless of visibility.
- Do not claim that every application will keep rendering while parked.

## Suggested implementation sequence

1. Build and run the Windows-only WGC probe in `tools/wgc-probe`. It lists top-level windows, lets the user select a parked HWND, and reports per-second frame counts and content size. Compare the same HWND while parked and on-stage; no UI integration yet.
2. Add the smallest public MyGo API needed to publish a native shared BGRA surface, reusing the existing D3D11 renderer import and keyed-mutex path.
3. Add a per-visible-card capture manager with bounded sessions and deterministic cleanup.
4. Wire the live image into the existing card window layer, leaving all current transforms intact.
5. Reuse the same texture in flights and validate frame-perfect hand-off.
6. Enable progressively after the capture, lifecycle, and performance tests pass.
