// Copyright 2022 Google Inc. All rights reserved
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

// This is the only source file for ckati-newer, a helper utility
// invoked by generated Ninja files to evaluate `$?`.

#include <stdbool.h>
#include <stdio.h>
#include <sys/stat.h>

double GetTimestamp(const char* filename) {
  struct stat st;
  if (stat(filename, &st) < 0) {
    return -2.0;
  }
#if defined(__linux__)
  return st.st_mtime + st.st_mtim.tv_nsec * 0.001 * 0.001 * 0.001;
#else
  return st.st_mtime;
#endif
}

int main(int argc, const char* argv[]) {
  if (argc < 2) {
    return 1;
  }
  double target_age = GetTimestamp(argv[1]);
  bool first = true;
  for (int i = 2; i < argc; i++) {
    if (GetTimestamp(argv[i]) > target_age) {
      if (first) {
        first = false;
      } else {
        putchar(' ');
      }
      puts(argv[i]);
    }
  }
  return 0;
}
