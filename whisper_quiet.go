package main

/*
#include "whisper.h"

static void whisper_noop_log(enum ggml_log_level level, const char *text, void *user_data) {
	(void)level;
	(void)text;
	(void)user_data;
}

static void whisper_suppress_logs(void) {
	whisper_log_set(whisper_noop_log, NULL);
}
*/
import "C"

func init() {
	C.whisper_suppress_logs()
}
