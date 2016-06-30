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

// +build ignore

#include "eval.h"

#include <errno.h>
#include <string.h>

#include "expr.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "parser.h"
#include "rule.h"
#include "stmt.h"
#include "strutil.h"
#include "symtab.h"
#include "var.h"

Evaluator::Evaluator()
    : last_rule_(NULL),
      current_scope_(NULL),
      avoid_io_(false),
      eval_depth_(0),
      posix_sym_(Intern(".POSIX")),
      is_posix_(false) {
}

Evaluator::~Evaluator() {
  // delete vars_;
  // for (auto p : rule_vars) {
  //   delete p.second;
  // }
}

Var* Evaluator::EvalRHS(Symbol lhs, Value* rhs_v, StringPiece orig_rhs,
                        AssignOp op, bool is_override) {
  VarOrigin origin = (
      (is_bootstrap_ ? VarOrigin::DEFAULT :
       is_commandline_ ? VarOrigin::COMMAND_LINE :
       is_override ? VarOrigin::OVERRIDE : VarOrigin::FILE));

  Var* rhs = NULL;
  bool needs_assign = true;
  switch (op) {
    case AssignOp::COLON_EQ: {
      SimpleVar* sv = new SimpleVar(origin);
      rhs_v->Eval(this, sv->mutable_value());
      rhs = sv;
      break;
    }
    case AssignOp::EQ:
      rhs = new RecursiveVar(rhs_v, origin, orig_rhs);
      break;
    case AssignOp::PLUS_EQ: {
      Var* prev = LookupVarInCurrentScope(lhs);
      if (!prev->IsDefined()) {
        rhs = new RecursiveVar(rhs_v, origin, orig_rhs);
      } else {
        prev->AppendVar(this, rhs_v);
        rhs = prev;
        needs_assign = false;
      }
      break;
    }
    case AssignOp::QUESTION_EQ: {
      Var* prev = LookupVarInCurrentScope(lhs);
      if (!prev->IsDefined()) {
        rhs = new RecursiveVar(rhs_v, origin, orig_rhs);
      } else {
        rhs = prev;
        needs_assign = false;
      }
      break;
    }
  }

  LOG("Assign: %s=%s", lhs.c_str(), rhs->DebugString().c_str());
  if (needs_assign) {
    return rhs;
  }
  return NULL;
}

void Evaluator::EvalAssign(const AssignStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;
  Symbol lhs = stmt->GetLhsSymbol(this);
  if (lhs.empty())
    Error("*** empty variable name.");
  Var* rhs = EvalRHS(lhs, stmt->rhs, stmt->orig_rhs, stmt->op,
                     stmt->directive == AssignDirective::OVERRIDE);
  if (rhs)
    lhs.SetGlobalVar(rhs,
                     stmt->directive == AssignDirective::OVERRIDE);
}

void Evaluator::EvalRule(const RuleStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;

  const string&& expr = stmt->expr->Eval(this);
  // See semicolon.mk.
  if (expr.find_first_not_of(" \t;") == string::npos) {
    if (stmt->term == ';')
      Error("*** missing rule before commands.");
    return;
  }

  Rule* rule;
  RuleVarAssignment rule_var;
  function<string()> after_term_fn = [this, stmt](){
    return stmt->after_term ? stmt->after_term->Eval(this) : "";
  };
  ParseRule(loc_, expr, stmt->term, after_term_fn, &rule, &rule_var);

  if (rule) {
    if (stmt->term == ';') {
      rule->cmds.push_back(stmt->after_term);
    }

    for (Symbol o : rule->outputs) {
      if (o == posix_sym_)
        is_posix_ = true;
    }

    LOG("Rule: %s", rule->DebugString().c_str());
    rules_.push_back(rule);
    last_rule_ = rule;
    return;
  }

  Symbol lhs = Intern(rule_var.lhs);
  for (Symbol output : rule_var.outputs) {
    auto p = rule_vars_.emplace(output, nullptr);
    if (p.second) {
      p.first->second = new Vars;
    }

    Value* rhs = stmt->after_term;
    if (!rule_var.rhs.empty()) {
      Value* lit = NewLiteral(rule_var.rhs);
      if (rhs) {
        // TODO: We always insert two whitespaces around the
        // terminator. Preserve whitespaces properly.
        if (stmt->term == ';') {
          rhs = NewExpr3(lit, NewLiteral(StringPiece(" ; ")), rhs);
        } else {
          rhs = NewExpr3(lit, NewLiteral(StringPiece(" = ")), rhs);
        }
      } else {
        rhs = lit;
      }
    }

    current_scope_ = p.first->second;
    Var* rhs_var = EvalRHS(lhs, rhs, StringPiece("*TODO*"), rule_var.op);
    if (rhs_var)
      current_scope_->Assign(lhs, new RuleVar(rhs_var, rule_var.op));
    current_scope_ = NULL;
  }
}

void Evaluator::EvalCommand(const CommandStmt* stmt) {
  loc_ = stmt->loc();

  if (!last_rule_) {
    vector<Stmt*> stmts;
    ParseNotAfterRule(stmt->orig, stmt->loc(), &stmts);
    for (Stmt* a : stmts)
      a->Eval(this);
    return;
  }

  last_rule_->cmds.push_back(stmt->expr);
  if (last_rule_->cmd_lineno == 0)
    last_rule_->cmd_lineno = stmt->loc().lineno;
  LOG("Command: %s", stmt->expr->DebugString().c_str());
}

void Evaluator::EvalIf(const IfStmt* stmt) {
  loc_ = stmt->loc();

  bool is_true;
  switch (stmt->op) {
    case CondOp::IFDEF:
    case CondOp::IFNDEF: {
      string var_name;
      stmt->lhs->Eval(this, &var_name);
      Symbol lhs = Intern(TrimRightSpace(var_name));
      if (lhs.str().find_first_of(" \t") != string::npos)
        Error("*** invalid syntax in conditional.");
      Var* v = LookupVarInCurrentScope(lhs);
      is_true = (v->String().empty() == (stmt->op == CondOp::IFNDEF));
      break;
    }
    case CondOp::IFEQ:
    case CondOp::IFNEQ: {
      const string&& lhs = stmt->lhs->Eval(this);
      const string&& rhs = stmt->rhs->Eval(this);
      is_true = ((lhs == rhs) == (stmt->op == CondOp::IFEQ));
      break;
    }
    default:
      CHECK(false);
      abort();
  }

  const vector<Stmt*>* stmts;
  if (is_true) {
    stmts = &stmt->true_stmts;
  } else {
    stmts = &stmt->false_stmts;
  }
  for (Stmt* a : *stmts) {
    LOG("%s", a->DebugString().c_str());
    a->Eval(this);
  }
}

void Evaluator::DoInclude(const string& fname) {
  Makefile* mk = MakefileCacheManager::Get()->ReadMakefile(fname);
  CHECK(mk->Exists());

  Var* var_list = LookupVar(Intern("MAKEFILE_LIST"));
  var_list->AppendVar(this, NewLiteral(Intern(TrimLeadingCurdir(fname)).str()));
  for (Stmt* stmt : mk->stmts()) {
    LOG("%s", stmt->DebugString().c_str());
    stmt->Eval(this);
  }
}

void Evaluator::EvalInclude(const IncludeStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;

  const string&& pats = stmt->expr->Eval(this);
  for (StringPiece pat : WordScanner(pats)) {
    ScopedTerminator st(pat);
    vector<string>* files;
    Glob(pat.data(), &files);

    if (stmt->should_exist) {
      if (files->empty()) {
        // TOOD: Kati does not support building a missing include file.
        Error(StringPrintf("%s: %s", pat.data(), strerror(errno)));
      }
    }

    for (const string& fname : *files) {
      if (!stmt->should_exist && g_flags.ignore_optional_include_pattern &&
          Pattern(g_flags.ignore_optional_include_pattern).Match(fname)) {
        continue;
      }
      DoInclude(fname);
    }
  }
}

void Evaluator::EvalExport(const ExportStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;

  const string&& exports = stmt->expr->Eval(this);
  for (StringPiece tok : WordScanner(exports)) {
    size_t equal_index = tok.find('=');
    if (equal_index == string::npos) {
      exports_[Intern(tok)] = stmt->is_export;
    } else if (equal_index == 0 ||
               (equal_index == 1 &&
                (tok[0] == ':' || tok[0] == '?' || tok[0] == '+'))) {
      // Do not export tokens after an assignment.
      break;
    } else {
      StringPiece lhs, rhs;
      AssignOp op;
      ParseAssignStatement(tok, equal_index, &lhs, &rhs, &op);
      exports_[Intern(lhs)] = stmt->is_export;
    }
  }
}

Var* Evaluator::LookupVarGlobal(Symbol name) {
  Var* v = name.GetGlobalVar();
  if (v->IsDefined())
    return v;
  used_undefined_vars_.insert(name);
  return v;
}

Var* Evaluator::LookupVar(Symbol name) {
  if (current_scope_) {
    Var* v = current_scope_->Lookup(name);
    if (v->IsDefined())
      return v;
  }
  return LookupVarGlobal(name);
}

Var* Evaluator::LookupVarInCurrentScope(Symbol name) {
  if (current_scope_) {
    return current_scope_->Lookup(name);
  }
  return LookupVarGlobal(name);
}

string Evaluator::EvalVar(Symbol name) {
  return LookupVar(name)->Eval(this);
}

string Evaluator::GetShell() {
  return EvalVar(kShellSym);
}

string Evaluator::GetShellFlag() {
  // TODO: Handle $(.SHELLFLAGS)
  return is_posix_ ? "-ec" : "-c";
}

string Evaluator::GetShellAndFlag() {
  string shell = GetShell();
  shell += ' ';
  shell += GetShellFlag();
  return shell;
}

void Evaluator::Error(const string& msg) {
  ERROR("%s:%d: %s", LOCF(loc_), msg.c_str());
}

unordered_set<Symbol> Evaluator::used_undefined_vars_;
