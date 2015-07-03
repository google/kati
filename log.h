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
#include "stringprintf.h"

using namespace std;

extern bool g_log_no_exit;
extern string* g_last_error;

// Useful for logging-only arguments.
#define UNUSED __attribute__((unused))

#ifdef NOLOG
#define LOG(args...)
#else
#define LOG(args...) do {                                           \
    fprintf(stderr, "*kati*: %s\n", StringPrintf(args).c_str());    \
  } while(0)
#endif

#define LOG_STAT(args...) do {                                      \
    if (g_enable_stat_logs)                                         \
      fprintf(stderr, "*kati*: %s\n", StringPrintf(args).c_str());  \
  } while(0)

#define PERROR(...) do {                                            \
    fprintf(stderr, "%s: %s\n", StringPrintf(__VA_ARGS__).c_str(),  \
            strerror(errno));                                       \
    exit(1);                                                        \
  } while (0)

#define WARN(...) do {                                          \
    fprintf(stderr, "%s\n", StringPrintf(__VA_ARGS__).c_str()); \
  } while (0)

#define ERROR(...) do {                                         \
    fprintf(stderr, "%s\n", StringPrintf(__VA_ARGS__).c_str()); \
    if (!g_log_no_exit)                                         \
      exit(1);                                                  \
    g_last_error = new string(StringPrintf(__VA_ARGS__));       \
  } while (0)

#define CHECK(c) if (!(c)) ERROR("%s:%d: %s", __FILE__, __LINE__, #c)

#endif  // LOG_H_
