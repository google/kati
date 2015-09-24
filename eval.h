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

#ifndef EVAL_H_
#define EVAL_H_

#include <memory>
#include <unordered_map>
#include <unordered_set>
#include <vector>

#include "loc.h"
#include "stmt.h"
#include "string_piece.h"
#include "symtab.h"

using namespace std;

class Makefile;
class Rule;
class Var;
class Vars;

struct EvalResult {
  ~EvalResult();
  vector<shared_ptr<Rule>> rules;
  Vars* vars;
  unordered_map<StringPiece, Vars*> rule_vars;
  // TODO: read_mks
  unordered_map<StringPiece, bool> exports;
};

class Evaluator {
 public:
  Evaluator(const Vars* vars);
  ~Evaluator();

  void EvalAssign(const AssignStmt* stmt);
  void EvalRule(const RuleStmt* stmt);
  void EvalCommand(const CommandStmt* stmt);
  void EvalIf(const IfStmt* stmt);
  void EvalInclude(const IncludeStmt* stmt);
  void EvalExport(const ExportStmt* stmt);

  Var* LookupVar(Symbol name);
  // For target specific variables.
  Var* LookupVarInCurrentScope(Symbol name);

  string EvalVar(Symbol name);

  const Loc& loc() const { return loc_; }
  void set_loc(const Loc& loc) { loc_ = loc; }

  const vector<shared_ptr<Rule>>& rules() const { return rules_; }
  const unordered_map<Symbol, Vars*>& rule_vars() const {
    return rule_vars_;
  }
  Vars* mutable_vars() { return vars_; }
  const unordered_map<Symbol, bool>& exports() const { return exports_; }

  void Error(const string& msg);

  void set_is_bootstrap(bool b) { is_bootstrap_ = b; }

  void set_current_scope(Vars* v) { current_scope_ = v; }

  bool avoid_io() const { return avoid_io_; }
  void set_avoid_io(bool a) { avoid_io_ = a; }

  const vector<string>& delayed_output_commands() const {
    return delayed_output_commands_;
  }
  void add_delayed_output_command(const string& c) {
    delayed_output_commands_.push_back(c);
  }
  void clear_delayed_output_commands() {
    delayed_output_commands_.clear();
  }

  static const unordered_set<Symbol>& used_undefined_vars() {
    return used_undefined_vars_;
  }

 private:
  Var* EvalRHS(Symbol lhs, Value* rhs, StringPiece orig_rhs, AssignOp op,
               bool is_override = false);
  void DoInclude(const string& fname);

  Var* LookupVarGlobal(Symbol name);

  const Vars* in_vars_;
  Vars* vars_;
  unordered_map<Symbol, Vars*> rule_vars_;
  vector<shared_ptr<Rule>> rules_;
  unordered_map<Symbol, bool> exports_;

  Rule* last_rule_;
  Vars* current_scope_;

  Loc loc_;
  bool is_bootstrap_;

  bool avoid_io_;
  // Commands which should run at ninja-time (i.e., info, warning, and
  // error).
  vector<string> delayed_output_commands_;

  static unordered_set<Symbol> used_undefined_vars_;
};

#endif  // EVAL_H_
