//go:build windows

package winapi

import (
	"syscall"
	"time"
)

var (
	procSendMessageWDrag = syscall.NewLazyDLL("user32.dll").NewProc("SendMessageW")
	procMouseEventDrag   = syscall.NewLazyDLL("user32.dll").NewProc("mouse_event")
)

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

// StartCaptionDrag restarts the user's drag as a new mouse gesture. The
// fullscreen move loop has already been cancelled and the window restored;
// synthesize the button-up/button-down edge that a manual release and
// re-press would produce before asking the window to enter its caption move
// loop. SendMessageW blocks until that loop ends, so run it off the UI thread.
func StartCaptionDrag(h HWND, p Point) bool {
	const (
		wmNCLButtonDown = 0x00A1
		htCaption       = 2
		mouseeventfLeftDown = 0x0002
		mouseeventfLeftUp   = 0x0004
	)
	if !IsWindow(h) {
		return false
	}

	// A held physical button does not create a new WM_*BUTTONDOWN transition.
	// Explicitly end the old gesture, then begin a fresh one at the current
	// pointer location, just as if the user had released and pressed again.
	procMouseEventDrag.Call(mouseeventfLeftUp, 0, 0, 0, 0)
	time.Sleep(15 * time.Millisecond)
	procMouseEventDrag.Call(mouseeventfLeftDown, 0, 0, 0, 0)

	lparam := uintptr(uint32(uint16(p.X)) | uint32(uint16(p.Y))<<16)
	go procSendMessageWDrag.Call(
		uintptr(h), wmNCLButtonDown, htCaption, lparam,
	)
	return true
}
