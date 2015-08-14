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

#ifndef COMMAND_H_
#define COMMAND_H_

#include <vector>

#include "symtab.h"

using namespace std;

struct DepNode;
class Evaluator;

struct Command {
  explicit Command(Symbol o)
      : output(o), echo(true), ignore_error(false) {
  }
  Symbol output;
  string cmd;
  bool echo;
  bool ignore_error;
};

class CommandEvaluator {
 public:
  explicit CommandEvaluator(Evaluator* ev);
  void Eval(DepNode* n, vector<Command*>* commands);
  const DepNode* current_dep_node() const { return current_dep_node_; }

 private:
  Evaluator* ev_;
  DepNode* current_dep_node_;
};

#endif  // COMMAND_H_
