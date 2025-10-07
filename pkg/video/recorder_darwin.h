#ifndef LIMITLESS_CONTEXT_RECORDER_DARWIN_H
#define LIMITLESS_CONTEXT_RECORDER_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

int recorder_initialize(void);
int recorder_record_screen(const char *path, double duration, char **error_out);
void recorder_cancel_active(void);
void recorder_free_string(char *ptr);

#ifdef __cplusplus
}
#endif

#endif
