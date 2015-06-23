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

#include "fileutil.h"

#include <errno.h>
#include <limits.h>
#include <sys/stat.h>
#include <unistd.h>

#include "log.h"

bool Exists(StringPiece filename) {
  CHECK(filename.size() < PATH_MAX);
  char buf[PATH_MAX+1];
  memcpy(buf, filename.data(), filename.size());
  buf[filename.size()] = 0;
  struct stat st;
  if (stat(buf, &st) < 0) {
    return false;
  }
  return true;
}
