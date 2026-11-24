//go:build windows

package winapi

import (
	"syscall"
	"unsafe"
)

var procPostMessageWDrag = syscall.NewLazyDLL("user32.dll").NewProc("PostMessageW")

// FullscreenRestoreRect reports the saved normal placement of a borderless
// window that currently covers its monitor. It deliberately fails closed:
// a maximized window, a window with a caption, or a window without a distinct
// saved normal rectangle is left to the ordinary drag path.
func FullscreenRestoreRect(h HWND) (Rect, Rect, bool) {
	if !IsWindow(h) || IsIconic(h) || IsZoomed(h) || Style(h)&WsCaption != 0 {
		return Rect{}, Rect{}, false
	}
	frame := FrameBounds(h)
	if frame.Empty() {
		return Rect{}, Rect{}, false
	}
	mon := MonitorInfo(MonitorFromRect(frame)).Monitor
	const tolerance int32 = 2
	if abs32(frame.Left-mon.Left) > tolerance ||
		abs32(frame.Top-mon.Top) > tolerance ||
		abs32(frame.Right-mon.Right) > tolerance ||
		abs32(frame.Bottom-mon.Bottom) > tolerance {
		return Rect{}, Rect{}, false
	}
	normal, maximized, ok := RestoredRect(h)
	if !ok || maximized || normal.Empty() {
		return Rect{}, Rect{}, false
	}
	// A fullscreen placement is not useful as a restore target. Requiring a
	// materially different rectangle avoids turning ordinary fullscreen
	// windows into a zero-distance "restore".
	if normal == frame || normal.W() >= frame.W()-4 && normal.H() >= frame.H()-4 {
		return Rect{}, Rect{}, false
	}
	return normal, frame, true
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// PostCaptionDrag posts the native non-client caption-down message. Posting,
// rather than sending, is essential: DefWindowProc enters a modal move loop
// and must not block Stage Manager's main thread.
func PostCaptionDrag(h HWND, p Point) bool {
	const (
		wmNCLButtonDown = 0x00A1
		htCaption       = 2
	)
	lparam := uintptr(uint32(uint16(p.X)) | uint32(uint16(p.Y))<<16)
	r, _, _ := procPostMessageWDrag.Call(
		uintptr(h), wmNCLButtonDown, htCaption, lparam,
	)
	_ = unsafe.Pointer(nil) // keep this file's Win32 payload representation explicit
	return r != 0
}
