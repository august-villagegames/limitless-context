//go:build darwin

package screenshots

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Foundation -framework CoreGraphics -framework ScreenCaptureKit -framework CoreImage -framework ImageIO -framework AVFoundation

#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreImage/CoreImage.h>
#import <ImageIO/ImageIO.h>
#import <CoreVideo/CoreVideo.h>
#import <CoreMedia/CoreMedia.h>
#import <dispatch/dispatch.h>
#import <stdlib.h>
#import <string.h>

struct CaptureBuffer {
        CFDataRef data;
        CFStringRef pixel_format;
        size_t width;
        size_t height;
        double scale;
        int used_sc;
        char *err;
};

static CFStringRef fourcc_to_string(FourCharCode code) {
        char buf[5];
        buf[0] = (code >> 24) & 0xFF;
        buf[1] = (code >> 16) & 0xFF;
        buf[2] = (code >> 8) & 0xFF;
        buf[3] = code & 0xFF;
        buf[4] = '\0';
        return CFStringCreateWithCString(NULL, buf, kCFStringEncodingASCII);
}

@interface SingleFrameCollector : NSObject<SCStreamOutput>
@property (nonatomic, strong) dispatch_semaphore_t semaphore;
@property (nonatomic, strong) NSData *imageData;
@property (nonatomic, strong) NSError *streamError;
@property (nonatomic, assign) size_t width;
@property (nonatomic, assign) size_t height;
@property (nonatomic, assign) FourCharCode pixelFormat;
@end

@implementation SingleFrameCollector
- (instancetype)init {
        if ((self = [super init])) {
                _semaphore = dispatch_semaphore_create(0);
        }
        return self;
}

- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer ofType:(SCStreamOutputType)type {
        if (type != SCStreamOutputTypeScreen) {
                return;
        }
        CVImageBufferRef buffer = CMSampleBufferGetImageBuffer(sampleBuffer);
        if (!buffer) {
                return;
        }
        CVPixelBufferLockBaseAddress(buffer, kCVPixelBufferLock_ReadOnly);
        size_t width = CVPixelBufferGetWidth(buffer);
        size_t height = CVPixelBufferGetHeight(buffer);
        FourCharCode pixelFormat = CVPixelBufferGetPixelFormatType(buffer);
        CIImage *ciImage = [CIImage imageWithCVImageBuffer:buffer];
        CIContext *context = [CIContext contextWithOptions:nil];
        CGRect rect = CGRectMake(0, 0, width, height);
        CGImageRef image = [context createCGImage:ciImage fromRect:rect];
        CVPixelBufferUnlockBaseAddress(buffer, kCVPixelBufferLock_ReadOnly);
        if (!image) {
                return;
        }
        CFMutableDataRef data = CFDataCreateMutable(NULL, 0);
        if (!data) {
                CGImageRelease(image);
                return;
        }
        CGImageDestinationRef dest = CGImageDestinationCreateWithData(data, CFSTR("public.png"), 1, NULL);
        if (!dest) {
                CFRelease(data);
                CGImageRelease(image);
                return;
        }
        CGImageDestinationAddImage(dest, image, NULL);
        if (!CGImageDestinationFinalize(dest)) {
                CFRelease(dest);
                CFRelease(data);
                CGImageRelease(image);
                return;
        }
        CFRelease(dest);
        CGImageRelease(image);
        self.imageData = (__bridge_transfer NSData *)data;
        self.width = width;
        self.height = height;
        self.pixelFormat = pixelFormat;
        dispatch_semaphore_signal(self.semaphore);
}

- (void)stream:(SCStream *)stream didStopWithError:(NSError *)error {
        self.streamError = error;
        dispatch_semaphore_signal(self.semaphore);
}
@end

static struct CaptureBuffer capture_with_cgwindow(void) {
        struct CaptureBuffer result = {0};
        CGRect bounds = CGRectInfinite;
        CGImageRef image = CGWindowListCreateImage(bounds, kCGWindowListOptionOnScreenOnly, kCGNullWindowID, kCGWindowImageDefault);
        if (!image) {
                result.err = strdup("cgwindow capture failed");
                return result;
        }
        CFMutableDataRef data = CFDataCreateMutable(NULL, 0);
        if (!data) {
                result.err = strdup("failed to allocate image buffer");
                CGImageRelease(image);
                return result;
        }
        CGImageDestinationRef dest = CGImageDestinationCreateWithData(data, CFSTR("public.png"), 1, NULL);
        if (!dest) {
                result.err = strdup("failed to create image destination");
                CFRelease(data);
                CGImageRelease(image);
                return result;
        }
        CGImageDestinationAddImage(dest, image, NULL);
        if (!CGImageDestinationFinalize(dest)) {
                result.err = strdup("failed to finalize image");
                CFRelease(dest);
                CFRelease(data);
                CGImageRelease(image);
                return result;
        }
        CFRelease(dest);
        result.data = data;
        result.width = CGImageGetWidth(image);
        result.height = CGImageGetHeight(image);
        result.scale = 1.0;
        result.pixel_format = CFRetain(CFSTR("CGImage"));
        result.used_sc = 0;
        CGImageRelease(image);
        return result;
}

static struct CaptureBuffer capture_with_screencapturekit(void) {
        struct CaptureBuffer result = {0};
        if (@available(macOS 12.3, *)) {
                NSError *contentError = nil;
                SCShareableContent *content = [SCShareableContent currentShareableContentWithError:&contentError];
                if (!content || content.displays.count == 0) {
                        if (contentError) {
                                result.err = strdup(contentError.localizedDescription.UTF8String);
                        }
                        return result;
                }
                SCDisplay *display = content.displays.firstObject;
                SCStreamConfiguration *config = [[SCStreamConfiguration alloc] init];
                config.showsCursor = YES;
                config.captureResolution = SCStreamCaptureResolutionAutomatic;
                config.pixelFormat = kCVPixelFormatType_32BGRA;
                config.colorSpaceName = (__bridge NSString *)kCGColorSpaceSRGB;
                config.width = display.width;
                config.height = display.height;

                SingleFrameCollector *collector = [[SingleFrameCollector alloc] init];
                NSError *streamError = nil;
                SCStreamFilter *filter = [[SCStreamFilter alloc] initWithDisplay:display excludingWindows:@[] exceptingApplications:@[]];
                SCStream *stream = [[SCStream alloc] initWithFilter:filter configuration:config delegate:nil];
                if (![stream addStreamOutput:collector type:SCStreamOutputTypeScreen sampleHandlerQueue:dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0) error:&streamError]) {
                        if (streamError) {
                                result.err = strdup(streamError.localizedDescription.UTF8String);
                        }
                        return result;
                }
                __block NSError *startError = nil;
                dispatch_semaphore_t startSem = dispatch_semaphore_create(0);
                [stream startCaptureWithCompletionHandler:^(NSError * _Nullable error) {
                        startError = error;
                        dispatch_semaphore_signal(startSem);
                }];
                dispatch_time_t startTimeout = dispatch_time(DISPATCH_TIME_NOW, (int64_t)(NSEC_PER_SEC));
                if (dispatch_semaphore_wait(startSem, startTimeout) != 0) {
                        result.err = strdup("timed out waiting to start ScreenCaptureKit stream");
                        [stream stopCaptureWithCompletionHandler:^(NSError * _Nullable error) {}];
                        return result;
                }
                if (startError) {
                        result.err = strdup(startError.localizedDescription.UTF8String);
                        [stream stopCaptureWithCompletionHandler:^(NSError * _Nullable error) {}];
                        return result;
                }
                dispatch_time_t captureTimeout = dispatch_time(DISPATCH_TIME_NOW, (int64_t)(NSEC_PER_SEC * 2));
                dispatch_semaphore_wait(collector.semaphore, captureTimeout);
                [stream stopCaptureWithCompletionHandler:^(NSError * _Nullable error) {}];
                if (collector.imageData) {
                        result.data = (__bridge_retained CFDataRef)collector.imageData;
                        result.width = collector.width;
                        result.height = collector.height;
                        result.scale = display.scaleFactor;
                        result.pixel_format = fourcc_to_string(collector.pixelFormat);
                        result.used_sc = 1;
                        return result;
                }
                if (collector.streamError) {
                        result.err = strdup(collector.streamError.localizedDescription.UTF8String);
                } else {
                        result.err = strdup("screen capture stream produced no frames");
                }
        }
        return result;
}

struct CaptureBuffer capture_screen_frame(void) {
        struct CaptureBuffer sc = capture_with_screencapturekit();
        if (sc.data != NULL) {
                return sc;
        }
        struct CaptureBuffer cg = capture_with_cgwindow();
        if (cg.data != NULL || cg.err != NULL) {
                if (sc.err != NULL) {
                        free(sc.err);
                }
                return cg;
        }
        return sc;
}

const UInt8 *capture_buffer_bytes(CFDataRef data) {
        return CFDataGetBytePtr(data);
}

CFIndex capture_buffer_length(CFDataRef data) {
        return CFDataGetLength(data);
}

char *cfstring_copy_utf8(CFStringRef str) {
        if (!str) {
                return NULL;
        }
        CFIndex length = CFStringGetLength(str);
        CFIndex size = CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1;
        char *buffer = malloc(size);
        if (buffer == NULL) {
                return NULL;
        }
        if (!CFStringGetCString(str, buffer, size, kCFStringEncodingUTF8)) {
                free(buffer);
                return NULL;
        }
        return buffer;
}
*/
import "C"

import (
	"context"
	"errors"
	"time"
	"unsafe"
)

type macCaptureProvider struct{}

func defaultCaptureProvider() (CaptureProvider, error) {
	return macCaptureProvider{}, nil
}

func (macCaptureProvider) Grab(ctx context.Context) (FrameCapture, error) {
	result := C.capture_screen_frame()
	if result.err != nil {
		defer C.free(unsafe.Pointer(result.err))
		return FrameCapture{}, errors.New(C.GoString(result.err))
	}
	if result.data == nil {
		return FrameCapture{}, errors.New("no image data returned from capture")
	}
	length := C.capture_buffer_length(result.data)
	bytesPtr := C.capture_buffer_bytes(result.data)
	png := C.GoBytes(unsafe.Pointer(bytesPtr), C.int(length))
	C.CFRelease(C.CFTypeRef(result.data))

	metadata := Metadata{
		CapturedAt: time.Now().UTC(),
		Backend:    "screencapturekit",
		Width:      int(result.width),
		Height:     int(result.height),
		Scale:      float64(result.scale),
	}
	if result.used_sc == 0 {
		metadata.Backend = "cgwindow"
	}
	if result.pixel_format != nil {
		str := C.cfstring_copy_utf8(result.pixel_format)
		if str != nil {
			metadata.PixelFormat = C.GoString(str)
			C.free(unsafe.Pointer(str))
		}
		C.CFRelease(C.CFTypeRef(result.pixel_format))
	}
	return FrameCapture{PNG: png, Metadata: metadata}, nil
}
