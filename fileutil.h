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

#include <memory>
#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

bool Exists(StringPiece f);
double GetTimestamp(StringPiece f);

int RunCommand(const string& shell, const string& cmd, bool redirect_stderr,
               string* out);

void Glob(const char* pat, vector<string>** files);

#endif  // FILEUTIL_H_
