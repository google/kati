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

#include "eval.h"

#include <errno.h>
#include <glob.h>
#include <string.h>

#include "ast.h"
#include "file.h"
#include "file_cache.h"
#include "rule.h"
#include "strutil.h"
#include "value.h"
#include "var.h"

EvalResult::~EvalResult() {
  for (auto p : rule_vars)
    delete p.second;
  delete vars;
}

Evaluator::Evaluator(const Vars* vars)
    : in_vars_(vars),
      vars_(new Vars()),
      last_rule_(NULL),
      current_scope_(NULL) {
}

Evaluator::~Evaluator() {
  // delete vars_;
  // for (auto p : rule_vars) {
  //   delete p.second;
  // }
}

Var* Evaluator::EvalRHS(StringPiece lhs, Value* rhs_v, StringPiece orig_rhs,
                        AssignOp op) {
  const char* origin = is_bootstrap_ ? "default" : "file";

  Var* rhs = NULL;
  bool needs_assign = true;
  switch (op) {
    case AssignOp::COLON_EQ:
      rhs = new SimpleVar(rhs_v->Eval(this), origin);
      break;
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

  LOG("Assign: %.*s=%s", SPF(lhs), rhs->DebugString().c_str());
  if (needs_assign) {
    return rhs;
  }
  return NULL;
}

void Evaluator::EvalAssign(const AssignAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;
  StringPiece lhs = Intern(*ast->lhs->Eval(this));
  Var* rhs = EvalRHS(lhs, ast->rhs, ast->orig_rhs, ast->op);
  if (rhs)
    vars_->Assign(lhs, rhs);
}

void Evaluator::EvalRule(const RuleAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  shared_ptr<string> expr = ast->expr->Eval(this);
  // See semicolon.mk.
  if (expr->find_first_not_of(" \t\n;") == string::npos)
    return;

  Rule* rule;
  RuleVarAssignment rule_var;
  ParseRule(loc_, *expr, ast->term == '=', &rule, &rule_var);

  if (rule) {
    LOG("Rule: %s", rule->DebugString().c_str());
    rules_.push_back(shared_ptr<Rule>(rule));
    last_rule_ = rule;
    return;
  }

  for (StringPiece output : rule_var.outputs) {
    auto p = rule_vars_.emplace(output, static_cast<Vars*>(NULL));
    if (p.second) {
      p.first->second = new Vars;
    }

    Value* rhs = ast->after_term;
    if (!rule_var.rhs.empty()) {
      Value* lit = NewLiteral(rule_var.rhs);
      if (rhs) {
        rhs = NewExpr2(lit, rhs);
      } else {
        rhs = lit;
      }
    }

    current_scope_ = p.first->second;
    StringPiece lhs = Intern(rule_var.lhs);
    Var* rhs_var = EvalRHS(lhs, rhs, STRING_PIECE("*TODO*"), rule_var.op);
    if (rhs_var)
      current_scope_->Assign(lhs, new RuleVar(rhs_var, rule_var.op));
    current_scope_ = NULL;
  }
}

void Evaluator::EvalCommand(const CommandAST* ast) {
  loc_ = ast->loc();

  if (!last_rule_) {
    // TODO:
    ERROR("TODO");
  }

  last_rule_->cmds.push_back(ast->expr);
  LOG("Command: %s", ast->expr->DebugString().c_str());
}

void Evaluator::EvalIf(const IfAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  bool is_true;
  switch (ast->op) {
    case CondOp::IFDEF:
    case CondOp::IFNDEF: {
      StringPiece lhs = Intern(*ast->lhs->Eval(this));
      Var* v = LookupVarInCurrentScope(lhs);
      shared_ptr<string> s = v->Eval(this);
      is_true = (s->empty() == (ast->op == CondOp::IFNDEF));
      break;
    }
    case CondOp::IFEQ:
    case CondOp::IFNEQ: {
      shared_ptr<string> lhs = ast->lhs->Eval(this);
      shared_ptr<string> rhs = ast->rhs->Eval(this);
      is_true = ((*lhs == *rhs) == (ast->op == CondOp::IFEQ));
      break;
    }
    default:
      CHECK(false);
  }

  const vector<AST*>* asts;
  if (is_true) {
    asts = &ast->true_asts;
  } else {
    asts = &ast->false_asts;
  }
  for (AST* a : *asts) {
    LOG("%s", a->DebugString().c_str());
    a->Eval(this);
  }
}

void Evaluator::DoInclude(const char* fname, bool should_exist) {
  Makefile* mk = MakefileCacheManager::Get()->ReadMakefile(fname);
  if (!mk->Exists()) {
    if (should_exist) {
      Error(StringPrintf(
          "%s: %s\n"
          "NOTE: kati does not support generating missing makefiles",
          fname, strerror(errno)));
    }
    return;
  }

  Var* var_list = LookupVar("MAKEFILE_LIST");
  var_list->AppendVar(this, NewLiteral(Intern(fname)));
  for (AST* ast : mk->asts()) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(this);
  }
}

void Evaluator::EvalInclude(const IncludeAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  shared_ptr<string> pats = ast->expr->Eval(this);
  for (StringPiece pat : WordScanner(*pats)) {
    ScopedTerminator st(pat);
    if (pat.find_first_of("?*[") != string::npos) {
      glob_t gl;
      glob(pat.data(), GLOB_NOSORT, NULL, &gl);
      for (size_t i = 0; i < gl.gl_pathc; i++) {
        DoInclude(gl.gl_pathv[i], ast->should_exist);
      }
      globfree(&gl);
    } else {
      DoInclude(pat.data(), ast->should_exist);
    }
  }
}

void Evaluator::EvalExport(const ExportAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  ERROR("TODO");
}

Var* Evaluator::LookupVar(StringPiece name) {
  if (current_scope_) {
    Var* v = current_scope_->Lookup(name);
    if (v->IsDefined())
      return v;
  }
  Var* v = vars_->Lookup(name);
  if (v->IsDefined())
    return v;
  return in_vars_->Lookup(name);
}

Var* Evaluator::LookupVarInCurrentScope(StringPiece name) {
  if (current_scope_) {
    return current_scope_->Lookup(name);
  }
  Var* v = vars_->Lookup(name);
  if (v->IsDefined())
    return v;
  return in_vars_->Lookup(name);
}

void Evaluator::Error(const string& msg) {
  ERROR("%s:%d: %s", LOCF(loc_), msg.c_str());
}
