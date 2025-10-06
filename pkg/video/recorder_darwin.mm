//go:build darwin

#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <string.h>
#import <stdlib.h>
#import "recorder_darwin.h"

@interface RecorderSampleWriter : NSObject <SCStreamOutput, AVCaptureVideoDataOutputSampleBufferDelegate>
@property(nonatomic, strong) AVAssetWriter *writer;
@property(nonatomic, strong) AVAssetWriterInput *videoInput;
@property(nonatomic) CMTime startTime;
@property(nonatomic) BOOL started;
@property(nonatomic) NSTimeInterval duration;
@property(nonatomic) dispatch_semaphore_t finishSemaphore;
@property(nonatomic) BOOL finished;
@property(nonatomic) BOOL cancelled;
@property(nonatomic, strong) NSError *error;
@property(nonatomic, copy) void (^stopHandler)(void);
@end

@implementation RecorderSampleWriter

- (instancetype)initWithURL:(NSURL *)url
                   duration:(NSTimeInterval)duration
                        size:(CGSize)size
                       error:(NSError **)error {
    self = [super init];
    if (!self) {
        return nil;
    }

    _duration = duration;
    _finishSemaphore = dispatch_semaphore_create(0);

    [[NSFileManager defaultManager] removeItemAtURL:url error:nil];

    NSDictionary *compression = @{AVVideoProfileLevelKey: AVVideoProfileLevelH264HighAutoLevel,
                                  AVVideoAverageBitRateKey: @(12 * 1000 * 1000)};
    NSDictionary *settings = @{AVVideoCodecKey: AVVideoCodecTypeH264,
                               AVVideoWidthKey: @(MAX(1.0, size.width)),
                               AVVideoHeightKey: @(MAX(1.0, size.height)),
                               AVVideoCompressionPropertiesKey: compression};

    _writer = [AVAssetWriter assetWriterWithURL:url fileType:AVFileTypeMPEG4 error:error];
    if (!_writer) {
        return nil;
    }

    _videoInput = [AVAssetWriterInput assetWriterInputWithMediaType:AVMediaTypeVideo outputSettings:settings];
    _videoInput.expectsMediaDataInRealTime = YES;

    if ([_writer canAddInput:_videoInput]) {
        [_writer addInput:_videoInput];
    } else {
        if (error) {
            *error = [NSError errorWithDomain:AVFoundationErrorDomain
                                         code:-1
                                     userInfo:@{NSLocalizedDescriptionKey : @"Unable to add video input"}];
        }
        return nil;
    }

    return self;
}

- (void)finishWithError:(NSError *)error {
    if (self.finished) {
        return;
    }
    self.finished = YES;
    if (error && !self.error) {
        self.error = error;
    }
    if (!self.started) {
        [self.writer cancelWriting];
        dispatch_semaphore_signal(self.finishSemaphore);
        if (self.stopHandler) {
            self.stopHandler();
        }
        return;
    }

    [self.videoInput markAsFinished];
    __block dispatch_semaphore_t semaphore = self.finishSemaphore;
    [self.writer finishWritingWithCompletionHandler:^{
        dispatch_semaphore_signal(semaphore);
    }];
    if (self.stopHandler) {
        self.stopHandler();
    }
}

- (void)processSampleBuffer:(CMSampleBufferRef)sampleBuffer {
    if (self.finished || self.cancelled) {
        return;
    }

    if (!CMSampleBufferDataIsReady(sampleBuffer)) {
        return;
    }

    CMTime presentation = CMSampleBufferGetPresentationTimeStamp(sampleBuffer);
    if (!self.started) {
        self.started = YES;
        self.startTime = presentation;
        if (![self.writer startWriting]) {
            NSError *error = self.writer.error ?: [NSError errorWithDomain:AVFoundationErrorDomain
                                                                      code:-2
                                                                  userInfo:@{NSLocalizedDescriptionKey : @"Failed to start AVAssetWriter"}];
            [self finishWithError:error];
            return;
        }
        [self.writer startSessionAtSourceTime:presentation];
    }

    CMTime relative = CMSampleBufferGetPresentationTimeStamp(sampleBuffer);
    double elapsed = CMTimeGetSeconds(CMTimeSubtract(relative, self.startTime));
    if (elapsed >= self.duration) {
        [self finishWithError:nil];
        return;
    }

    if (self.videoInput.readyForMoreMediaData) {
        if (![self.videoInput appendSampleBuffer:sampleBuffer]) {
            NSError *error = self.writer.error ?: [NSError errorWithDomain:AVFoundationErrorDomain
                                                                      code:-3
                                                                  userInfo:@{NSLocalizedDescriptionKey : @"Failed to append sample buffer"}];
            [self finishWithError:error];
        }
    }
}

- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer ofType:(SCStreamOutputType)type API_AVAILABLE(macos(12.3)) {
    if (type != SCStreamOutputTypeScreen) {
        return;
    }
    [self processSampleBuffer:sampleBuffer];
}

- (void)captureOutput:(AVCaptureOutput *)output didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer fromConnection:(AVCaptureConnection *)connection {
    [self processSampleBuffer:sampleBuffer];
}

- (void)cancelWithError:(NSError *)error {
    if (self.cancelled) {
        return;
    }
    self.cancelled = YES;
    [self finishWithError:error];
}

@end

@interface RecorderController : NSObject
@property(nonatomic, strong) RecorderSampleWriter *writer;
@property(nonatomic, strong) SCStream *stream API_AVAILABLE(macos(12.3));
@property(nonatomic) dispatch_queue_t streamQueue;
@property(nonatomic, strong) AVCaptureSession *session;
@property(nonatomic, strong) AVCaptureVideoDataOutput *videoOutput;
@property(nonatomic) dispatch_semaphore_t stopSemaphore;
@end

@implementation RecorderController

- (BOOL)recordToURL:(NSURL *)url duration:(NSTimeInterval)duration error:(NSError **)error {
    if (@available(macOS 12.3, *)) {
        return [self recordWithScreenCaptureKitToURL:url duration:duration error:error];
    }
    return [self recordWithAVFoundationToURL:url duration:duration error:error];
}

- (BOOL)recordWithScreenCaptureKitToURL:(NSURL *)url duration:(NSTimeInterval)duration error:(NSError **)error API_AVAILABLE(macos(12.3)) {
    __block SCShareableContent *content = nil;
    __block NSError *contentError = nil;
    dispatch_semaphore_t sema = dispatch_semaphore_create(0);
    [SCShareableContent currentShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable shareableContent, NSError * _Nullable err) {
        content = shareableContent;
        contentError = err;
        dispatch_semaphore_signal(sema);
    }];
    dispatch_semaphore_wait(sema, DISPATCH_TIME_FOREVER);
    if (!content) {
        if (error) {
            *error = contentError ?: [NSError errorWithDomain:SCStreamErrorDomain code:-1 userInfo:@{NSLocalizedDescriptionKey : @"Unable to enumerate displays"}];
        }
        return NO;
    }

    SCDisplay *display = content.displays.firstObject;
    for (SCDisplay *candidate in content.displays) {
        if (candidate.isMainDisplay) {
            display = candidate;
            break;
        }
    }

    if (!display) {
        if (error) {
            *error = [NSError errorWithDomain:SCStreamErrorDomain code:-2 userInfo:@{NSLocalizedDescriptionKey : @"No displays available"}];
        }
        return NO;
    }

    CGSize size = CGSizeMake(display.width, display.height);
    NSError *writerError = nil;
    self.writer = [[RecorderSampleWriter alloc] initWithURL:url duration:duration size:size error:&writerError];
    if (!self.writer) {
        if (error) {
            *error = writerError;
        }
        return NO;
    }

    self.streamQueue = dispatch_queue_create("com.limitless-context.recorder.stream", DISPATCH_QUEUE_SERIAL);
    self.stopSemaphore = dispatch_semaphore_create(0);

    __weak typeof(self) weakSelf = self;
    self.writer.stopHandler = ^{
        __strong typeof(self) strongSelf = weakSelf;
        if (!strongSelf) {
            return;
        }
        [strongSelf stopStream];
    };

    SCStreamConfiguration *configuration = [[SCStreamConfiguration alloc] init];
    configuration.width = size.width;
    configuration.height = size.height;
    configuration.minimumFrameInterval = CMTimeMake(1, 60);
    configuration.queueDepth = 8;
    configuration.showsCursor = YES;
    configuration.colorSpaceName = kCGColorSpaceSRGB;
    configuration.pixelFormat = kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange;

    self.stream = [[SCStream alloc] initWithFilter:display configuration:configuration delegate:nil];

    NSError *outputError = nil;
    if (![self.stream addStreamOutput:self.writer type:SCStreamOutputTypeScreen sampleHandlerQueue:self.streamQueue error:&outputError]) {
        if (error) {
            *error = outputError ?: [NSError errorWithDomain:SCStreamErrorDomain code:-3 userInfo:@{NSLocalizedDescriptionKey : @"Failed to add stream output"}];
        }
        return NO;
    }

    dispatch_semaphore_t startSemaphore = dispatch_semaphore_create(0);
    __block NSError *startError = nil;
    [self.stream startCaptureWithCompletionHandler:^(NSError * _Nullable err) {
        startError = err;
        dispatch_semaphore_signal(startSemaphore);
    }];
    dispatch_semaphore_wait(startSemaphore, DISPATCH_TIME_FOREVER);
    if (startError) {
        if (error) {
            *error = startError;
        }
        return NO;
    }

    dispatch_semaphore_wait(self.writer.finishSemaphore, DISPATCH_TIME_FOREVER);
    dispatch_semaphore_wait(self.stopSemaphore, DISPATCH_TIME_FOREVER);

    if (self.writer.error && error) {
        *error = self.writer.error;
    }

    return self.writer.error == nil;
}

- (void)stopStream API_AVAILABLE(macos(12.3)) {
    if (!self.stream) {
        dispatch_semaphore_signal(self.stopSemaphore);
        return;
    }
    __block NSError *stopError = nil;
    dispatch_semaphore_t stopSemaphore = dispatch_semaphore_create(0);
    [self.stream stopCaptureWithCompletionHandler:^(NSError * _Nullable err) {
        stopError = err;
        dispatch_semaphore_signal(stopSemaphore);
    }];
    dispatch_semaphore_wait(stopSemaphore, DISPATCH_TIME_FOREVER);
    if (stopError && !self.writer.error) {
        self.writer.error = stopError;
    }
    dispatch_semaphore_signal(self.stopSemaphore);
}

- (BOOL)recordWithAVFoundationToURL:(NSURL *)url duration:(NSTimeInterval)duration error:(NSError **)error {
    CGDirectDisplayID displayID = CGMainDisplayID();
    CGSize size = CGSizeMake(CGDisplayPixelsWide(displayID), CGDisplayPixelsHigh(displayID));

    NSError *writerError = nil;
    self.writer = [[RecorderSampleWriter alloc] initWithURL:url duration:duration size:size error:&writerError];
    if (!self.writer) {
        if (error) {
            *error = writerError;
        }
        return NO;
    }

    self.stopSemaphore = dispatch_semaphore_create(0);

    __weak typeof(self) weakSelf = self;
    self.writer.stopHandler = ^{
        __strong typeof(self) strongSelf = weakSelf;
        if (!strongSelf) {
            return;
        }
        [strongSelf stopSession];
    };

    self.session = [[AVCaptureSession alloc] init];
    self.session.sessionPreset = AVCaptureSessionPresetHigh;

    AVCaptureScreenInput *screenInput = [[AVCaptureScreenInput alloc] initWithDisplayID:displayID];
    screenInput.minFrameDuration = CMTimeMake(1, 60);
    screenInput.capturesCursor = YES;
    screenInput.capturesMouseClicks = YES;

    if ([self.session canAddInput:screenInput]) {
        [self.session addInput:screenInput];
    } else {
        if (error) {
            *error = [NSError errorWithDomain:AVFoundationErrorDomain code:-10 userInfo:@{NSLocalizedDescriptionKey : @"Unable to add screen input"}];
        }
        return NO;
    }

    self.videoOutput = [[AVCaptureVideoDataOutput alloc] init];
    self.videoOutput.alwaysDiscardsLateVideoFrames = NO;
    NSDictionary *videoSettings = @{(NSString *)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange)};
    self.videoOutput.videoSettings = videoSettings;

    dispatch_queue_t queue = dispatch_queue_create("com.limitless-context.recorder.capture", DISPATCH_QUEUE_SERIAL);
    [self.videoOutput setSampleBufferDelegate:self.writer queue:queue];

    if ([self.session canAddOutput:self.videoOutput]) {
        [self.session addOutput:self.videoOutput];
    } else {
        if (error) {
            *error = [NSError errorWithDomain:AVFoundationErrorDomain code:-11 userInfo:@{NSLocalizedDescriptionKey : @"Unable to add video output"}];
        }
        return NO;
    }

    [self.session startRunning];

    dispatch_semaphore_wait(self.writer.finishSemaphore, DISPATCH_TIME_FOREVER);
    dispatch_semaphore_wait(self.stopSemaphore, DISPATCH_TIME_FOREVER);

    if (self.writer.error && error) {
        *error = self.writer.error;
    }

    return self.writer.error == nil;
}

- (void)stopSession {
    if (!self.session) {
        dispatch_semaphore_signal(self.stopSemaphore);
        return;
    }
    [self.session stopRunning];
    dispatch_semaphore_signal(self.stopSemaphore);
}

- (void)cancel {
    NSError *cancelError = [NSError errorWithDomain:NSCocoaErrorDomain code:NSUserCancelledError userInfo:nil];
    [self.writer cancelWithError:cancelError];
}

@end

static RecorderController *gActiveController;
static dispatch_queue_t gStateQueue;

static char *recorder_copy_cstring(NSString *string) {
    if (!string) {
        return NULL;
    }
    const char *utf8 = [string UTF8String];
    if (!utf8) {
        return NULL;
    }
    size_t length = strlen(utf8) + 1;
    char *buffer = malloc(length);
    if (!buffer) {
        return NULL;
    }
    memcpy(buffer, utf8, length);
    return buffer;
}

int recorder_initialize(void) {
    @autoreleasepool {
        if (!gStateQueue) {
            gStateQueue = dispatch_queue_create("com.limitless-context.recorder.state", DISPATCH_QUEUE_SERIAL);
        }
        return 0;
    }
}

int recorder_record_screen(const char *path, double duration, char **error_out) {
    @autoreleasepool {
        if (!gStateQueue) {
            gStateQueue = dispatch_queue_create("com.limitless-context.recorder.state", DISPATCH_QUEUE_SERIAL);
        }
        if (!path) {
            if (error_out) {
                *error_out = recorder_copy_cstring(@"destination path is required");
            }
            return -1;
        }
        NSString *filePath = [NSString stringWithUTF8String:path];
        NSURL *url = [NSURL fileURLWithPath:filePath];
        RecorderController *controller = [[RecorderController alloc] init];
        __block RecorderController *previous = nil;
        dispatch_sync(gStateQueue, ^{
            previous = gActiveController;
            gActiveController = controller;
        });
        if (previous) {
            [previous cancel];
        }

        NSError *error = nil;
        BOOL success = [controller recordToURL:url duration:duration error:&error];

        dispatch_sync(gStateQueue, ^{
            gActiveController = nil;
        });

        if (!success) {
            if (error_out && error) {
                *error_out = recorder_copy_cstring(error.localizedDescription ?: @"recording failed");
            }
            return -1;
        }
        return 0;
    }
}

void recorder_cancel_active(void) {
    @autoreleasepool {
        dispatch_sync(gStateQueue, ^{
            [gActiveController cancel];
        });
    }
}

void recorder_free_string(char *ptr) {
    if (ptr) {
        free(ptr);
    }
}
