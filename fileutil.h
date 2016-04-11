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

#ifndef FILEUTIL_H_
#define FILEUTIL_H_

#include <errno.h>

#include <memory>
#include <string>
#include <unordered_map>
#include <vector>

#include "string_piece.h"

using namespace std;

bool Exists(StringPiece f);
double GetTimestampFromStat(const struct stat& st);
double GetTimestamp(StringPiece f);

enum struct RedirectStderr {
  NONE,
  STDOUT,
  DEV_NULL,
};

int RunCommand(const string& shell, const string& cmd,
               RedirectStderr redirect_stderr,
               string* out);

void GetExecutablePath(string* path);

void Glob(const char* pat, vector<string>** files);

const unordered_map<string, vector<string>*>& GetAllGlobCache();

void ClearGlobCache();

#define HANDLE_EINTR(x) ({                                \
      int r;                                              \
      do {                                                \
        r = (x);                                          \
      } while (r == -1 && errno == EINTR);                \
      r;                                                  \
    })

#endif  // FILEUTIL_H_
