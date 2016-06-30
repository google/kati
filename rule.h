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
#include <vector>

#include "loc.h"
#include "log.h"
#include "stmt.h"
#include "string_piece.h"
#include "symtab.h"

using namespace std;

class Value;

class Rule {
 public:
  Rule();

  Loc cmd_loc() const { return Loc(loc.filename, cmd_lineno); }

  string DebugString() const;

  vector<Symbol> outputs;
  vector<Symbol> inputs;
  vector<Symbol> order_only_inputs;
  vector<Symbol> output_patterns;
  bool is_double_colon;
  bool is_suffix_rule;
  vector<Value*> cmds;
  Loc loc;
  int cmd_lineno;

 private:
  void Error(const string& msg) {
    ERROR("%s:%d: %s", loc.filename, loc.lineno, msg.c_str());
  }
};

struct RuleVarAssignment {
  vector<Symbol> outputs;
  StringPiece lhs;
  StringPiece rhs;
  AssignOp op;
};

// If |rule| is not NULL, |rule_var| is filled. If the expression
// after the terminator |term| is needed (this happens only when
// |term| is '='), |after_term_fn| will be called to obtain the right
// hand side.
void ParseRule(Loc& loc, StringPiece line, char term,
               function<string()> after_term_fn,
               Rule** rule, RuleVarAssignment* rule_var);

#endif  // RULE_H_
