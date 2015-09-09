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

#include "ast.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "parser.h"
#include "rule.h"
#include "strutil.h"
#include "symtab.h"
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
      current_scope_(NULL),
      avoid_io_(false) {
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
       is_override ? VarOrigin::OVERRIDE : VarOrigin::FILE));

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

  LOG("Assign: %s=%s", lhs.c_str(), rhs->DebugString().c_str());
  if (needs_assign) {
    return rhs;
  }
  return NULL;
}

void Evaluator::EvalAssign(const AssignAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;
  Symbol lhs = Intern(ast->lhs->Eval(this));
  if (lhs.empty())
    Error("*** empty variable name.");
  Var* rhs = EvalRHS(lhs, ast->rhs, ast->orig_rhs, ast->op,
                     ast->directive == AssignDirective::OVERRIDE);
  if (rhs)
    vars_->Assign(lhs, rhs);
}

void Evaluator::EvalRule(const RuleAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  const string&& expr = ast->expr->Eval(this);
  // See semicolon.mk.
  if (expr.find_first_not_of(" \t\n;") == string::npos)
    return;

  Rule* rule;
  RuleVarAssignment rule_var;
  ParseRule(loc_, expr, ast->term, &rule, &rule_var);

  if (rule) {
    if (ast->term == ';') {
      rule->cmds.push_back(ast->after_term);
    }

    LOG("Rule: %s", rule->DebugString().c_str());
    rules_.push_back(shared_ptr<Rule>(rule));
    last_rule_ = rule;
    return;
  }

  for (Symbol output : rule_var.outputs) {
    auto p = rule_vars_.emplace(output, nullptr);
    if (p.second) {
      p.first->second = new Vars;
    }

    Value* rhs = ast->after_term;
    if (!rule_var.rhs.empty()) {
      Value* lit = NewLiteral(rule_var.rhs);
      if (rhs) {
        // TODO: We always insert two whitespaces around the
        // terminator. Preserve whitespaces properly.
        if (ast->term == ';') {
          rhs = NewExpr3(lit, NewLiteral(StringPiece(" ; ")), rhs);
        } else {
          rhs = NewExpr3(lit, NewLiteral(StringPiece(" = ")), rhs);
        }
      } else {
        rhs = lit;
      }
    }

    current_scope_ = p.first->second;
    Symbol lhs = Intern(rule_var.lhs);
    Var* rhs_var = EvalRHS(lhs, rhs, StringPiece("*TODO*"), rule_var.op);
    if (rhs_var)
      current_scope_->Assign(lhs, new RuleVar(rhs_var, rule_var.op));
    current_scope_ = NULL;
  }
}

void Evaluator::EvalCommand(const CommandAST* ast) {
  loc_ = ast->loc();

  if (!last_rule_) {
    vector<AST*> asts;
    ParseNotAfterRule(ast->orig, ast->loc(), &asts);
    for (AST* a : asts)
      a->Eval(this);
    return;
  }

  last_rule_->cmds.push_back(ast->expr);
  if (last_rule_->cmd_lineno == 0)
    last_rule_->cmd_lineno = ast->loc().lineno;
  LOG("Command: %s", ast->expr->DebugString().c_str());
}

void Evaluator::EvalIf(const IfAST* ast) {
  loc_ = ast->loc();

  bool is_true;
  switch (ast->op) {
    case CondOp::IFDEF:
    case CondOp::IFNDEF: {
      Symbol lhs = Intern(ast->lhs->Eval(this));
      Var* v = LookupVarInCurrentScope(lhs);
      const string&& s = v->Eval(this);
      is_true = (s.empty() == (ast->op == CondOp::IFNDEF));
      break;
    }
    case CondOp::IFEQ:
    case CondOp::IFNEQ: {
      const string&& lhs = ast->lhs->Eval(this);
      const string&& rhs = ast->rhs->Eval(this);
      is_true = ((lhs == rhs) == (ast->op == CondOp::IFEQ));
      break;
    }
    default:
      CHECK(false);
      abort();
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

void Evaluator::DoInclude(const string& fname) {
  Makefile* mk = MakefileCacheManager::Get()->ReadMakefile(fname);
  CHECK(mk->Exists());

  Var* var_list = LookupVar(Intern("MAKEFILE_LIST"));
  var_list->AppendVar(this, NewLiteral(Intern(TrimLeadingCurdir(fname)).str()));
  for (AST* ast : mk->asts()) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(this);
  }
}

void Evaluator::EvalInclude(const IncludeAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  const string&& pats = ast->expr->Eval(this);
  for (StringPiece pat : WordScanner(pats)) {
    ScopedTerminator st(pat);
    vector<string>* files;
    Glob(pat.data(), &files);

    if (ast->should_exist) {
      if (files->empty()) {
        Error(StringPrintf(
            "%s: %s\n"
            "NOTE: kati does not support generating missing makefiles",
            pat.data(), strerror(errno)));
      }
    }

    for (const string& fname : *files) {
      if (!ast->should_exist && g_flags.ignore_optional_include_pattern &&
          Pattern(g_flags.ignore_optional_include_pattern).Match(fname)) {
        return;
      }
      DoInclude(fname);
    }
  }
}

void Evaluator::EvalExport(const ExportAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  const string&& exports = ast->expr->Eval(this);
  for (StringPiece tok : WordScanner(exports)) {
    size_t equal_index = tok.find('=');
    if (equal_index == string::npos) {
      exports_[Intern(tok)] = ast->is_export;
    } else if (equal_index == 0 ||
               (equal_index == 1 &&
                (tok[0] == ':' || tok[0] == '?' || tok[0] == '+'))) {
      // Do not export tokens after an assignment.
      break;
    } else {
      StringPiece lhs, rhs;
      AssignOp op;
      ParseAssignStatement(tok, equal_index, &lhs, &rhs, &op);
      exports_[Intern(lhs)] = ast->is_export;
    }
  }
}

Var* Evaluator::LookupVarGlobal(Symbol name) {
  Var* v = vars_->Lookup(name);
  if (v->IsDefined())
    return v;
  v = in_vars_->Lookup(name);
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

void Evaluator::Error(const string& msg) {
  ERROR("%s:%d: %s", LOCF(loc_), msg.c_str());
}

unordered_set<Symbol> Evaluator::used_undefined_vars_;
