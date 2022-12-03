// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#ifndef LOG_H_
#define LOG_H_

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "flags.h"
#include "log.h"
#include "stringprintf.h"

extern bool g_log_no_exit;
extern std::string* g_last_error;

#define SPF(s) static_cast<int>((s).size()), (s).data()

// Useful for logging-only arguments.
#define UNUSED __attribute__((unused))

#ifdef NOLOG
#define LOG(args...)
#else
#define LOG(args...)                                             \
  do {                                                           \
    fprintf(stderr, "*kati*: %s\n", StringPrintf(args).c_str()); \
  } while (0)
#endif

#define LOG_STAT(args...)                                          \
  do {                                                             \
    if (g_flags.enable_stat_logs)                                  \
      fprintf(stderr, "*kati*: %s\n", StringPrintf(args).c_str()); \
  } while (0)

#define PLOG(...)                                                  \
  do {                                                             \
    fprintf(stderr, "%s: %s\n", StringPrintf(__VA_ARGS__).c_str(), \
            strerror(errno));                                      \
  } while (0)

#define PERROR(...)    \
  do {                 \
    PLOG(__VA_ARGS__); \
    exit(1);           \
  } while (0)

#define WARN(...)                                               \
  do {                                                          \
    fprintf(stderr, "%s\n", StringPrintf(__VA_ARGS__).c_str()); \
  } while (0)

#define KATI_WARN(...)                                            \
  do {                                                            \
    if (g_flags.enable_kati_warnings)                             \
      fprintf(stderr, "%s\n", StringPrintf(__VA_ARGS__).c_str()); \
  } while (0)

#define ERROR(...)                                                \
  do {                                                            \
    if (!g_log_no_exit) {                                         \
      fprintf(stderr, "%s\n", StringPrintf(__VA_ARGS__).c_str()); \
      exit(1);                                                    \
    }                                                             \
    g_last_error = new std::string(StringPrintf(__VA_ARGS__));    \
  } while (0)

#define CHECK(c) \
  if (!(c))      \
  ERROR("%s:%d: %s", __FILE__, __LINE__, #c)

// Set of logging functions that will automatically colorize lines that have
// location information when --color_warnings is set.
void ColorWarnLog(const char* file, int line, const char* msg);
void ColorErrorLog(const char* file, int line, const char* msg);

#define WARN_LOC(loc, ...)                                      \
  do {                                                          \
    ColorWarnLog(LOCF(loc), StringPrintf(__VA_ARGS__).c_str()); \
  } while (0)

#define KATI_WARN_LOC(loc, ...)                                   \
  do {                                                            \
    if (g_flags.enable_kati_warnings)                             \
      ColorWarnLog(LOCF(loc), StringPrintf(__VA_ARGS__).c_str()); \
  } while (0)

#define ERROR_LOC(loc, ...)                                      \
  do {                                                           \
    ColorErrorLog(LOCF(loc), StringPrintf(__VA_ARGS__).c_str()); \
  } while (0)

#endif  // LOG_H_
