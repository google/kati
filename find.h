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
#include <unordered_set>
#include <vector>

#include "string_piece.h"

using namespace std;

class FindCond;

enum struct FindCommandType {
  FIND,
  FINDLEAVES,
  LS,
};

struct FindCommand {
  FindCommand();
  ~FindCommand();

  bool Parse(const string& cmd);

  FindCommandType type;
  string chdir;
  string testdir;
  vector<string> finddirs;
  bool follows_symlinks;
  unique_ptr<FindCond> print_cond;
  unique_ptr<FindCond> prune_cond;
  int depth;
  int mindepth;
  bool redirect_to_devnull;

  unique_ptr<vector<string>> found_files;
  unique_ptr<unordered_set<string>> read_dirs;

 private:
  FindCommand(const FindCommand&) = delete;
  void operator=(FindCommand) = delete;
};

class FindEmulator {
 public:
  virtual ~FindEmulator() = default;

  virtual bool HandleFind(const string& cmd, const FindCommand& fc,
                          string* out) = 0;

  static FindEmulator* Get();

 protected:
  FindEmulator() = default;
};

void InitFindEmulator();

#endif  // FIND_H_
