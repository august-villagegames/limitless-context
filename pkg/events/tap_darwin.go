//go:build darwin

package events

/*
#cgo darwin CFLAGS: -x objective-c -fmodules -fobjc-arc
#cgo darwin LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework Cocoa
#include <ApplicationServices/ApplicationServices.h>
#include <Cocoa/Cocoa.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdint.h>

static Boolean axCheckTrusted(void) {
        const void *keys[] = { kAXTrustedCheckOptionPrompt };
        const void *values[] = { kCFBooleanTrue };
        CFDictionaryRef options = CFDictionaryCreate(kCFAllocatorDefault, keys, values, 1,
                                                     &kCFTypeDictionaryKeyCallBacks,
                                                     &kCFTypeDictionaryValueCallBacks);
        Boolean trusted = AXIsProcessTrustedWithOptions(options);
        CFRelease(options);
        return trusted;
}

extern CGEventRef goHandleEvent(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *userInfo);

static CFRunLoopSourceRef startEventTap(uintptr_t handle, CGEventMask mask, CFMachPortRef *tapOut) {
        CFMachPortRef tap = CGEventTapCreate(kCGSessionEventTap,
                                             kCGHeadInsertEventTap,
                                             kCGEventTapOptionListenOnly,
                                             mask,
                                             goHandleEvent,
                                             (void *)handle);
        if (tap == NULL) {
                return NULL;
        }
        CGEventTapEnable(tap, true);
        CFRunLoopSourceRef source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
        *tapOut = tap;
        return source;
}

static CFRunLoopRef currentRunLoop(void) {
        return CFRunLoopGetCurrent();
}

static CGEventMask cgEventMaskBit(CGEventType type) {
        return ((CGEventMask)1) << type;
}

static void addSourceToRunLoop(CFRunLoopRef loop, CFRunLoopSourceRef source) {
        CFRunLoopAddSource(loop, source, kCFRunLoopCommonModes);
}

static void runCurrentRunLoop(void) {
        CFRunLoopRun();
}

static void stopRunLoop(CFRunLoopRef loop) {
        CFRunLoopStop(loop);
}

static double cgEventGetX(CGEventRef event) {
        CGPoint point = CGEventGetLocation(event);
        return point.x;
}

static double cgEventGetY(CGEventRef event) {
        CGPoint point = CGEventGetLocation(event);
        return point.y;
}

static int64_t cgEventGetKeycode(CGEventRef event) {
        return CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
}

static CFStringRef copyFocusedWindowTitle(void) {
        AXUIElementRef systemWide = AXUIElementCreateSystemWide();
        if (systemWide == NULL) {
                return NULL;
        }
        AXUIElementRef app = NULL;
        AXError err = AXUIElementCopyAttributeValue(systemWide, kAXFocusedApplicationAttribute, (CFTypeRef *)&app);
        if (err != kAXErrorSuccess || app == NULL) {
                if (app != NULL) {
                        CFRelease(app);
                }
                CFRelease(systemWide);
                return NULL;
        }
        AXUIElementRef window = NULL;
        err = AXUIElementCopyAttributeValue(app, kAXFocusedWindowAttribute, (CFTypeRef *)&window);
        if (err != kAXErrorSuccess || window == NULL) {
                if (window != NULL) {
                        CFRelease(window);
                }
                CFRelease(app);
                CFRelease(systemWide);
                return NULL;
        }
        CFStringRef title = NULL;
        err = AXUIElementCopyAttributeValue(window, kAXTitleAttribute, (CFTypeRef *)&title);
        if (window != NULL) {
                CFRelease(window);
        }
        CFRelease(app);
        CFRelease(systemWide);
        return title;
}

static CFStringRef copyFocusedAppBundle(void) {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) {
                return NULL;
        }
        NSString *bundleID = app.bundleIdentifier ?: @"";
        return (__bridge_retained CFStringRef)bundleID;
}

static CFStringRef copyFocusedAppName(void) {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) {
                return NULL;
        }
        NSString *name = app.localizedName ?: @"";
        return (__bridge_retained CFStringRef)name;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/cgo"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

type macEventSource struct {
	now func() time.Time
}

func defaultEventSource(opts Options, clock func() time.Time) EventSource {
	return &macEventSource{now: clock}
}

type macEventStream struct {
	emit       func(Event) error
	now        func() time.Time
	loop       C.CFRunLoopRef
	stopped    chan struct{}
	stopLoop   func()
	err        error
	focusLock  sync.Mutex
	lastApp    string
	lastBundle string
	lastTitle  string
	closeOnce  sync.Once
}

func newMacEventStream(now func() time.Time, emit func(Event) error) *macEventStream {
	return &macEventStream{
		emit:    emit,
		now:     now,
		stopped: make(chan struct{}),
	}
}

func (s *macEventStream) close() {
	s.closeOnce.Do(func() {
		close(s.stopped)
	})
}

func (s *macEventStream) setErr(err error) {
	if err == nil {
		return
	}
	if s.err == nil {
		s.err = err
	}
}

func (s *macEventStream) emitEvent(event Event) {
	if s.err != nil {
		return
	}
	if err := s.emit(event); err != nil {
		s.setErr(err)
		if s.stopLoop != nil {
			s.stopLoop()
		}
	}
}

func (s *macEventStream) emitFocus(now time.Time) {
	meta := make(map[string]string)
	if title := cfStringToGo(C.copyFocusedWindowTitle()); title != "" {
		meta["title"] = title
	}
	if name := cfStringToGo(C.copyFocusedAppName()); name != "" {
		meta["app"] = name
	}
	if bundle := cfStringToGo(C.copyFocusedAppBundle()); bundle != "" {
		meta["bundle"] = bundle
	}

	s.focusLock.Lock()
	changed := meta["app"] != s.lastApp || meta["bundle"] != s.lastBundle || meta["title"] != s.lastTitle
	if changed {
		s.lastApp = meta["app"]
		s.lastBundle = meta["bundle"]
		s.lastTitle = meta["title"]
	}
	s.focusLock.Unlock()

	if !changed {
		return
	}

	event := Event{
		Timestamp: now,
		Category:  "window",
		Action:    "focus",
		Target:    meta["bundle"],
		Metadata:  trimEmpty(meta),
	}
	s.emitEvent(event)
}

func (s *macEventStream) handleKeyboard(now time.Time, eventType C.CGEventType, event C.CGEventRef) {
	keycode := int(C.cgEventGetKeycode(event))
	meta := s.focusMetadata()
	meta["keycode"] = strconv.Itoa(keycode)
	action := "press"
	switch eventType {
	case C.kCGEventKeyUp:
		action = "release"
	case C.kCGEventFlagsChanged:
		action = "modifier"
	}
	s.emitEvent(Event{
		Timestamp: now,
		Category:  "keyboard",
		Action:    action,
		Target:    fmt.Sprintf("key:%d", keycode),
		Metadata:  meta,
	})
}

func (s *macEventStream) handleMouse(now time.Time, eventType C.CGEventType, event C.CGEventRef) {
	meta := s.focusMetadata()
	x := float64(C.cgEventGetX(event))
	y := float64(C.cgEventGetY(event))
	meta["x"] = fmt.Sprintf("%.2f", x)
	meta["y"] = fmt.Sprintf("%.2f", y)

	action := "move"
	switch eventType {
	case C.kCGEventLeftMouseDown:
		action = "left-down"
	case C.kCGEventLeftMouseUp:
		action = "left-up"
	case C.kCGEventRightMouseDown:
		action = "right-down"
	case C.kCGEventRightMouseUp:
		action = "right-up"
	case C.kCGEventOtherMouseDown:
		action = "other-down"
	case C.kCGEventOtherMouseUp:
		action = "other-up"
	case C.kCGEventScrollWheel:
		action = "scroll"
	}

	s.emitEvent(Event{
		Timestamp: now,
		Category:  "mouse",
		Action:    action,
		Target:    meta["app"],
		Metadata:  meta,
	})
}

func (s *macEventStream) focusMetadata() map[string]string {
	meta := make(map[string]string)
	s.focusLock.Lock()
	if s.lastApp != "" {
		meta["app"] = s.lastApp
	}
	if s.lastBundle != "" {
		meta["bundle"] = s.lastBundle
	}
	if s.lastTitle != "" {
		meta["title"] = s.lastTitle
	}
	s.focusLock.Unlock()
	return meta
}

func trimEmpty(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	cleaned := make(map[string]string, len(meta))
	for k, v := range meta {
		if v == "" {
			continue
		}
		cleaned[k] = v
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func cfStringToGo(str C.CFStringRef) string {
	if str == 0 {
		return ""
	}
	defer C.CFRelease(C.CFTypeRef(str))
	length := C.CFStringGetLength(str)
	if length == 0 {
		return ""
	}
	bufSize := C.CFIndex(1 + 4*length)
	buf := make([]byte, int(bufSize))
	if C.CFStringGetCString(str, (*C.char)(unsafe.Pointer(&buf[0])), bufSize, C.kCFStringEncodingUTF8) == C.Boolean(0) {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
}

func (s *macEventSource) Stream(ctx context.Context, emit func(Event) error) error {
	if C.axCheckTrusted() == C.Boolean(0) {
		return ErrAccessibilityPermission
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	stream := newMacEventStream(s.now, emit)
	handle := cgo.NewHandle(stream)
	defer handle.Delete()

	mask := C.cgEventMaskBit(C.kCGEventKeyDown) |
		C.cgEventMaskBit(C.kCGEventKeyUp) |
		C.cgEventMaskBit(C.kCGEventFlagsChanged) |
		C.cgEventMaskBit(C.kCGEventLeftMouseDown) |
		C.cgEventMaskBit(C.kCGEventLeftMouseUp) |
		C.cgEventMaskBit(C.kCGEventRightMouseDown) |
		C.cgEventMaskBit(C.kCGEventRightMouseUp) |
		C.cgEventMaskBit(C.kCGEventOtherMouseDown) |
		C.cgEventMaskBit(C.kCGEventOtherMouseUp) |
		C.cgEventMaskBit(C.kCGEventMouseMoved) |
		C.cgEventMaskBit(C.kCGEventScrollWheel)

	var tap C.CFMachPortRef
	source := C.startEventTap(C.uintptr_t(handle), mask, &tap)
	if source == 0 {
		return errors.New("failed to create CGEvent tap")
	}
	defer C.CFRelease(C.CFTypeRef(source))
	defer C.CFRelease(C.CFTypeRef(tap))

	loop := C.currentRunLoop()
	stream.loop = loop
	stopOnce := sync.Once{}
	stream.stopLoop = func() {
		stopOnce.Do(func() {
			C.stopRunLoop(loop)
		})
	}
	C.addSourceToRunLoop(loop, source)

	cancelWatcher := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			stream.stopLoop()
		case <-stream.stopped:
		}
		close(cancelWatcher)
	}()

	stream.emitFocus(stream.now().UTC())
	C.runCurrentRunLoop()
	stream.stopLoop()
	stream.close()
	<-cancelWatcher
	if stream.err != nil {
		return stream.err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

//export goHandleEvent
func goHandleEvent(_ C.CGEventTapProxy, eventType C.CGEventType, event C.CGEventRef, userInfo unsafe.Pointer) C.CGEventRef {
	handle := cgo.Handle(uintptr(userInfo))
	streamIface := handle.Value()
	stream, ok := streamIface.(*macEventStream)
	if !ok {
		return event
	}

	now := stream.now().UTC()
	stream.emitFocus(now)

	switch eventType {
	case C.kCGEventKeyDown, C.kCGEventKeyUp, C.kCGEventFlagsChanged:
		stream.handleKeyboard(now, eventType, event)
	case C.kCGEventLeftMouseDown, C.kCGEventLeftMouseUp,
		C.kCGEventRightMouseDown, C.kCGEventRightMouseUp,
		C.kCGEventOtherMouseDown, C.kCGEventOtherMouseUp,
		C.kCGEventMouseMoved, C.kCGEventScrollWheel:
		stream.handleMouse(now, eventType, event)
	default:
		// ignore other events
	}

	return event
}
