//go:build windows

package winapi

import (
	"syscall"
)

var procSendMessageWDrag = syscall.NewLazyDLL("user32.dll").NewProc("SendMessageW")

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

// StartCaptionDrag asks the target window to process a real non-client
// button-down. DefWindowProc starts the native move loop while handling this
// message. That call does not return until the user releases the mouse, so it
// must run on another goroutine rather than blocking Stage Manager's UI loop.
// A posted WM_NCLBUTTONDOWN is not sufficient here: applications can ignore
// synthetic posted mouse messages instead of entering the move loop.
func StartCaptionDrag(h HWND, p Point) bool {
	const (
		wmNCLButtonDown = 0x00A1
		htCaption       = 2
	)
	if !IsWindow(h) {
		return false
	}
	lparam := uintptr(uint32(uint16(p.X)) | uint32(uint16(p.Y))<<16)
	go procSendMessageWDrag.Call(
		uintptr(h), wmNCLButtonDown, htCaption, lparam,
	)
	return true
}
