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

#include <assert.h>
#include <string_view>

#include "log.h"

bool g_failed;

#define ASSERT_BOOL(a, b)                                                     \
  do {                                                                        \
    bool A = (a);                                                             \
    if ((A) != (b)) {                                                         \
      fprintf(stderr, "Assertion failure at %s:%d: %s (which is %s) vs %s\n", \
              __FILE__, __LINE__, #a, (A ? "true" : "false"), #b);            \
      g_failed = true;                                                        \
    }                                                                         \
  } while (0)

#define ASSERT_EQ(a, b)                                                     \
  do {                                                                      \
    if ((a) != (b)) {                                                       \
      fprintf(stderr,                                                       \
              "Assertion failure at %s:%d: %s (which is \"%.*s\") vs %s\n", \
              __FILE__, __LINE__, #a, SPF(GetStringPiece(a)), #b);          \
      g_failed = true;                                                      \
    }                                                                       \
  } while (0)

std::string_view GetStringPiece(std::string_view s) {
  return s;
}
std::string_view GetStringPiece(size_t v) {
  static char buf[64];
  sprintf(buf, "%zd", v);
  return buf;
}
