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

#ifndef FIND_H_
#define FIND_H_

#include <memory>
#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

class FindCond;

struct FindCommand {
  FindCommand();
  ~FindCommand() = default;

  StringPiece chdir;
  StringPiece testdir;
  vector<StringPiece> finddirs;
  bool follows_symlinks;
  unique_ptr<FindCond> print_cond;
  unique_ptr<FindCond> prune_cond;
  int depth;
};

class FindEmulator {
 public:
  virtual ~FindEmulator() = default;

  virtual bool HandleFind(const string& cmd, string* out) = 0;

  static FindEmulator* Get();

 protected:
  FindEmulator() = default;
};

void InitFindEmulator();

#endif  // FIND_H_
