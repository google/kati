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

// +build ignore

#include "log.h"

#include "flags.h"
#include "strutil.h"

#define BOLD "\033[1m"
#define RESET "\033[0m"
#define MAGENTA "\033[35m"
#define RED "\033[31m"

void ColorErrorLog(const char* file, int line, const char* msg) {
  if (file == nullptr) {
    ERROR("%s", msg);
    return;
  }

  if (g_flags.color_warnings) {
    std::string_view filtered = TrimPrefix(msg, "*** ");

    ERROR(BOLD "%s:%d: " RED "error: " RESET BOLD "%s" RESET, file, line,
          std::string(filtered).c_str());
  } else {
    ERROR("%s:%d: %s", file, line, msg);
  }
}

void ColorWarnLog(const char* file, int line, const char* msg) {
  if (file == nullptr) {
    fprintf(stderr, "%s\n", msg);
    return;
  }

  if (g_flags.color_warnings) {
    std::string_view filtered = TrimPrefix(msg, "*warning*: ");
    filtered = TrimPrefix(filtered, "warning: ");

    fprintf(stderr,
            BOLD "%s:%d: " MAGENTA "warning: " RESET BOLD "%s" RESET "\n", file,
            line, std::string(filtered).c_str());
  } else {
    fprintf(stderr, "%s:%d: %s\n", file, line, msg);
  }
}

bool g_log_no_exit;
std::string* g_last_error;
