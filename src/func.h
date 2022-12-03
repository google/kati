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

#ifndef FUNC_H_
#define FUNC_H_

#include <memory>
#include <string>
#include <vector>

#include "expr.h"
#include "loc.h"

struct FuncInfo {
  const char* name;
  void (*func)(const std::vector<Value*>& args, Evaluator* ev, std::string* s);
  int arity;
  int min_arity;
  // For all parameters.
  bool trim_space;
  // Only for the first parameter.
  bool trim_right_space_1st;
};

const FuncInfo* GetFuncInfo(std::string_view name);

struct FindCommand;

enum struct CommandOp {
  SHELL,
  FIND,
  READ,
  READ_MISSING,
  WRITE,
  APPEND,
};

struct CommandResult {
  CommandOp op;
  std::string shell;
  std::string shellflag;
  std::string cmd;
  std::unique_ptr<FindCommand> find;
  std::string result;
  Loc loc;
};

const std::vector<CommandResult*>& GetShellCommandResults();

#endif  // FUNC_H_
