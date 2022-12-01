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

#include "stringprintf.h"

#include <assert.h>
#include <stdarg.h>

std::string StringPrintf(const char* format, ...) {
  std::string str;
  str.resize(128);
  for (int i = 0; i < 2; i++) {
    va_list args;
    va_start(args, format);
    int ret = vsnprintf(&str[0], str.size(), format, args);
    va_end(args);
    assert(ret >= 0);
    if (static_cast<size_t>(ret) < str.size()) {
      str.resize(ret);
      return str;
    }
    str.resize(ret + 1);
  }
  assert(false);
  __builtin_unreachable();
}
