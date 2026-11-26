//go:build windows

package winapi

import (
	"unsafe"
)

const (
	inputMouse       = 0
	mouseEventLeftUp = 0x0004
)

type mouseInput struct {
	Dx, Dy    int32
	Data      uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type syntheticInput struct {
	Type  uint32
	Mouse mouseInput
}


// ReleaseLeftButton releases the physical left-button drag with a real
// injected mouse input. It is a no-op when the button is already up.
// Ending the user's native drag before hiding or parking its HWND prevents
// the still-held button from continuing to move the window after WM_CANCELMODE.
func ReleaseLeftButton() bool {
	const keyDown = 0x8000
	state, _, _ := procGetAsyncKeyState.Call(0x01)
	if state&keyDown == 0 {
		return true
	}
	in := syntheticInput{
		Type: inputMouse,
		Mouse: mouseInput{Flags: mouseEventLeftUp},
	}
	sent, _, _ := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	return sent == 1
}
