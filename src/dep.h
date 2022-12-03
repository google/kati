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
#include <string_view>
#include <unordered_map>
#include <vector>

#include "loc.h"
#include "symtab.h"

class Evaluator;
class Rule;
class Value;
class Var;
class Vars;
class Frame;

typedef std::pair<Symbol, struct DepNode*> NamedDepNode;

struct DepNode {
  DepNode(Symbol output, bool is_phony, bool is_restat);
  std::string DebugString();

  Symbol output;
  std::vector<Value*> cmds;
  std::vector<NamedDepNode> deps;
  std::vector<NamedDepNode> order_onlys;
  std::vector<NamedDepNode> validations;
  bool has_rule;
  bool is_default_target;
  bool is_phony;
  bool is_restat;
  std::vector<Symbol> implicit_outputs;
  std::vector<Symbol> symlink_outputs;
  std::vector<Symbol> actual_inputs;
  std::vector<Symbol> actual_order_only_inputs;
  std::vector<Symbol> actual_validations;
  Vars* rule_vars;
  Var* depfile_var;
  Var* ninja_pool_var;
  Symbol output_pattern;
  Loc loc;
};

void MakeDep(Evaluator* ev,
             const std::vector<const Rule*>& rules,
             const std::unordered_map<Symbol, Vars*>& rule_vars,
             const std::vector<Symbol>& targets,
             std::vector<NamedDepNode>* nodes);

bool IsSpecialTarget(Symbol output);

#endif  // DEP_H_
