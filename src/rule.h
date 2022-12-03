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

#ifndef RULE_H_
#define RULE_H_

#include <functional>
#include <string>
#include <string_view>
#include <vector>

#include "loc.h"
#include "log.h"
#include "stmt.h"
#include "symtab.h"

class Value;

class Rule {
 public:
  Rule();

  Loc cmd_loc() const { return Loc(loc.filename, cmd_lineno); }

  std::string DebugString() const;

  void ParseInputs(const std::string_view& inputs_string);

  void ParsePrerequisites(const std::string_view& line,
                          size_t pos,
                          const RuleStmt* rule_stmt);

  static bool IsPatternRule(const std::string_view& target_string) {
    return target_string.find('%') != std::string::npos;
  }

  std::vector<Symbol> outputs;
  std::vector<Symbol> inputs;
  std::vector<Symbol> order_only_inputs;
  std::vector<Symbol> output_patterns;
  std::vector<Symbol> validations;
  bool is_double_colon;
  bool is_suffix_rule;
  std::vector<Value*> cmds;
  Loc loc;
  int cmd_lineno;

 private:
  void Error(const std::string& msg) { ERROR_LOC(loc, "%s", msg.c_str()); }
};

#endif  // RULE_H_
