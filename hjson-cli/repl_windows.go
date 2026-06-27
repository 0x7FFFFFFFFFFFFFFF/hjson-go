//go:build windows

package main

import (
	"bufio"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

// Console input mode flags (see SetConsoleMode docs).
const (
	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
)

const keyEventType = 0x0001

const (
	rightCtrlPressed = 0x0004
	leftCtrlPressed  = 0x0008
)

const (
	vkBack   = 0x08
	vkReturn = 0x0D
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode    = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode    = kernel32.NewProc("SetConsoleMode")
	procReadConsoleInputW = kernel32.NewProc("ReadConsoleInputW")
)

// keyEventRecord mirrors the memory layout of a Windows INPUT_RECORD that holds
// a KEY_EVENT_RECORD. The struct is 20 bytes, matching INPUT_RECORD; for other
// event types the fields are ignored (we check eventType first).
type keyEventRecord struct {
	eventType       uint16
	_               uint16
	keyDown         int32
	repeatCount     uint16
	virtualKeyCode  uint16
	virtualScanCode uint16
	unicodeChar     uint16
	controlKeyState uint32
}

var (
	consoleHandle  syscall.Handle
	savedMode      uint32
	savedModeValid bool
)

func getConsoleMode(h syscall.Handle) (uint32, bool) {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode)))
	return mode, r != 0
}

func setConsoleMode(h syscall.Handle, mode uint32) {
	procSetConsoleMode.Call(uintptr(h), uintptr(mode))
}

func readConsoleInput(h syscall.Handle, rec *keyEventRecord) bool {
	var n uint32
	r, _, _ := procReadConsoleInputW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(rec)),
		1,
		uintptr(unsafe.Pointer(&n)),
	)
	return r != 0 && n > 0
}

// restoreConsole restores the original console mode (echo and line input) that
// was changed for raw key reading. Safe to call multiple times.
func restoreConsole() {
	if savedModeValid {
		setConsoleMode(consoleHandle, savedMode)
		savedModeValid = false
	}
}

// replReadLoop reads raw console key events so that Ctrl+C can be used to submit
// the current input without terminating the process. Each Ctrl+C submits the
// buffered input for conversion; Ctrl+Z quits. Input is echoed manually since
// echo is disabled while reading raw key events.
func replReadLoop(submit chan<- string, done chan<- struct{}) {
	consoleHandle = syscall.Handle(os.Stdin.Fd())
	mode, ok := getConsoleMode(consoleHandle)
	if !ok {
		// stdin is not a real console; fall back to a plain line reader.
		replReadLoopFallback(submit, done)
		return
	}
	savedMode = mode
	savedModeValid = true
	setConsoleMode(consoleHandle, mode&^(enableProcessedInput|enableLineInput|enableEchoInput))

	var line []rune
	var lastWasCR bool
	var rec keyEventRecord
	for {
		if !readConsoleInput(consoleHandle, &rec) {
			done <- struct{}{}
			return
		}
		if rec.eventType != keyEventType || rec.keyDown == 0 {
			continue
		}
		ctrl := rec.controlKeyState&(leftCtrlPressed|rightCtrlPressed) != 0
		ch := rune(rec.unicodeChar)
		for i := uint16(0); i < rec.repeatCount; i++ {
			switch {
			case ch == 0x03 || (ctrl && rec.virtualKeyCode == 'C'):
				// Ctrl+C: submit the current input and keep going.
				submit <- string(line)
				line = line[:0]
				lastWasCR = false
			case ch == 0x1a || (ctrl && rec.virtualKeyCode == 'Z'):
				// Ctrl+Z: quit.
				done <- struct{}{}
				return
			case rec.virtualKeyCode == vkReturn || ch == '\r' || ch == '\n':
				// Collapse a CRLF pair into a single newline.
				if ch == '\n' && lastWasCR {
					lastWasCR = false
					continue
				}
				lastWasCR = ch == '\r' || rec.virtualKeyCode == vkReturn
				line = append(line, '\n')
				os.Stderr.WriteString("\r\n")
			case rec.virtualKeyCode == vkBack:
				lastWasCR = false
				if n := len(line); n > 0 {
					line = line[:n-1]
					os.Stderr.WriteString("\b \b")
				}
			case ch == '\t':
				lastWasCR = false
				line = append(line, '\t')
				os.Stderr.WriteString("\t")
			case ch >= 0x20:
				lastWasCR = false
				line = append(line, ch)
				var b [utf8.UTFMax]byte
				n := utf8.EncodeRune(b[:], ch)
				os.Stderr.Write(b[:n])
			}
		}
	}
}

// replReadLoopFallback is used when stdin is not a real console. It reads all
// input until EOF and submits it once.
func replReadLoopFallback(submit chan<- string, done chan<- struct{}) {
	reader := bufio.NewReader(os.Stdin)
	var sb strings.Builder
	for {
		line, err := reader.ReadString('\n')
		sb.WriteString(line)
		if err != nil {
			if sb.Len() > 0 {
				submit <- sb.String()
			}
			done <- struct{}{}
			return
		}
	}
}
