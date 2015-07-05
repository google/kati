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
#include <vector>

#include "ast.h"
#include "loc.h"
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

  void EvalAssign(const AssignAST* ast);
  void EvalRule(const RuleAST* ast);
  void EvalCommand(const CommandAST* ast);
  void EvalIf(const IfAST* ast);
  void EvalInclude(const IncludeAST* ast);
  void EvalExport(const ExportAST* ast);

  Var* LookupVar(Symbol name);
  // For target specific variables.
  Var* LookupVarInCurrentScope(Symbol name);

  shared_ptr<string> EvalVar(Symbol name);

  const Loc& loc() const { return loc_; }

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

 private:
  Var* EvalRHS(Symbol lhs, Value* rhs, StringPiece orig_rhs, AssignOp op,
               bool is_override = false);
  void DoInclude(const string& fname);

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
};

#endif  // EVAL_H_
