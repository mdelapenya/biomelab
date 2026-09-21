// focus-trace observes foreground transitions on an interactive Windows desktop.
// It never activates a window or records keystrokes, titles or command lines.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type message struct {
	hwnd           uintptr
	id             uint32
	wparam, lparam uintptr
	timestamp      uint32
	x, y           int32
	private        uint32
}

func main() {
	duration := flag.Duration("duration", 5*time.Minute, "recording duration")
	output := flag.String("out", "focus.csv", "new CSV file to create")
	flag.Parse()
	if err := record(*output, *duration); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func record(path string, duration time.Duration) error {
	if duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	defer writer.Flush()
	if err := writer.Write([]string{"observed_utc", "event_uptime_ms", "hwnd", "pid"}); err != nil {
		return err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	setHook := user32.NewProc("SetWinEventHook")
	unhook := user32.NewProc("UnhookWinEvent")
	getPID := user32.NewProc("GetWindowThreadProcessId")
	getMessage := user32.NewProc("GetMessageW")
	translateMessage := user32.NewProc("TranslateMessage")
	dispatchMessage := user32.NewProc("DispatchMessageW")
	peekMessage := user32.NewProc("PeekMessageW")
	postThreadMessage := user32.NewProc("PostThreadMessageW")
	getForeground := user32.NewProc("GetForegroundWindow")
	writeEvent := func(hwnd, eventTime uintptr) {
		var pid uint32
		_, _, _ = getPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		_ = writer.Write([]string{time.Now().UTC().Format(time.RFC3339Nano), strconv.FormatUint(uint64(eventTime), 10), fmt.Sprintf("0x%x", hwnd), strconv.FormatUint(uint64(pid), 10)})
		writer.Flush()
	}
	callback := windows.NewCallback(func(_ uintptr, _ uint32, hwnd uintptr, _ int32, _ int32, _ uint32, eventTime uint32) uintptr {
		writeEvent(hwnd, uintptr(eventTime))
		return 0
	})
	// EVENT_SYSTEM_FOREGROUND; WINEVENT_OUTOFCONTEXT delivers callbacks on
	// this thread's message loop, without injecting into observed processes.
	hook, _, hookErr := setHook.Call(3, 3, 0, callback, 0, 0, 0)
	if hook == 0 {
		return fmt.Errorf("SetWinEventHook: %w", hookErr)
	}
	defer func() { _, _, _ = unhook.Call(hook) }()

	var msg message
	// Create the message queue before the timer can post WM_QUIT.
	_, _, _ = peekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
	threadID := windows.GetCurrentThreadId()
	timer := time.AfterFunc(duration, func() { _, _, _ = postThreadMessage.Call(uintptr(threadID), 0x12, 0, 0) })
	defer timer.Stop()
	foreground, _, _ := getForeground.Call()
	writeEvent(foreground, 0) // initial snapshot has no originating event timestamp
	fmt.Fprintf(os.Stderr, "Recording foreground transitions to %s for %s. Switch to your working application.\n", path, duration)
	for {
		result, _, getErr := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("GetMessage: %w", getErr)
		}
		if result == 0 {
			return writer.Error()
		}
		_, _, _ = translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
