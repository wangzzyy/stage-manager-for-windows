# Windows Graphics Capture probe

This small Windows-only diagnostic captures one top-level HWND with Windows Graphics Capture (WGC) and reports how many frames arrive each second. It does not modify Stage Manager or the target window.

## Build

Requires Visual Studio 2022 C++ tools and a Windows 10 SDK (10.0.19041 or newer).

```powershell
cmake -S tools/wgc-probe -B build/wgc-probe -A x64
cmake --build build/wgc-probe --config Release
```

## Test a parked window

1. Start Stage Manager and move a test window to a non-active stage so Stage Manager parks it off the virtual desktop.
2. Run `build/wgc-probe/Release/wgc-probe.exe` without arguments.
3. Find the target in the printed window list. Parked windows should still appear with their off-screen rectangle and a non-minimized state.
4. Enter its HWND in hexadecimal, such as `0x001A03F2`.
5. Compare the reported per-second frame counts while the window is parked and while it is on-stage.

You can also pass a handle directly:

```powershell
wgc-probe.exe --hwnd 0x001A03F2 --seconds 30
```

## Interpretation

- `total_frames=0`: WGC did not deliver a frame. Check OS support, HWND validity, capture access, and whether the target is minimized.
- A steady nonzero `delta` while parked: WGC is receiving updates from this app in the parked state.
- Frames only while on-stage: the app or Windows capture path is suspending updates while the window is off-screen.
- A lower parked frame rate: the source may be throttling its own rendering. WGC does not force an application to render at the monitor refresh rate.

This probe deliberately stops at frame-arrival validation. It does not yet copy frames into a shared texture or connect them to the UI renderer.
