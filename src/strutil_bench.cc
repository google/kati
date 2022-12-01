// Copyright 2016 Google Inc. All rights reserved
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

#include <string>
#include <vector>

#include "flags.h"
#include "string_piece.h"
#include "strutil.h"
#include "timeutil.h"

int main() {
  g_flags.enable_stat_logs = true;
  std::string s;
  while (s.size() < 400000) {
    if (!s.empty())
      s += ' ';
    s += "frameworks/base/docs/html/tv/adt-1/index.jd";
  }

  ScopedTimeReporter tr("WordScanner");
  static const int N = 1000;
  for (int i = 0; i < N; i++) {
    std::vector<StringPiece> toks;
    WordScanner(s).Split(&toks);
  }
}
