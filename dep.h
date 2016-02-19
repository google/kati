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

#ifndef DEP_H_
#define DEP_H_

#include <string>
#include <unordered_map>
#include <vector>

#include "loc.h"
#include "string_piece.h"
#include "symtab.h"

class Evaluator;
class Rule;
class Value;
class Var;
class Vars;

struct DepNode {
  DepNode(Symbol output, bool is_phony, bool is_restat);

  Symbol output;
  vector<Value*> cmds;
  vector<DepNode*> deps;
  vector<DepNode*> order_onlys;
  vector<DepNode*> parents;
  bool has_rule;
  bool is_default_target;
  bool is_phony;
  bool is_restat;
  vector<Symbol> actual_inputs;
  vector<Symbol> actual_order_only_inputs;
  Vars* rule_vars;
  Var* depfile_var;
  Symbol output_pattern;
  Loc loc;
};

void InitDepNodePool();
void QuitDepNodePool();

void MakeDep(Evaluator* ev,
             const vector<const Rule*>& rules,
             const unordered_map<Symbol, Vars*>& rule_vars,
             const vector<Symbol>& targets,
             vector<DepNode*>* nodes);

#endif  // DEP_H_
