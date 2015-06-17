#include "eval.h"

#include "ast.h"
#include "file.h"
#include "rule.h"
#include "strutil.h"
#include "value.h"
#include "var.h"

EvalResult::~EvalResult() {
  for (Rule* r : rules)
    delete r;
  for (auto p : rule_vars)
    delete p.second;
  delete vars;
}

Evaluator::Evaluator(const Vars* vars)
    : in_vars_(vars),
      vars_(new Vars()),
      last_rule_(NULL) {
}

Evaluator::~Evaluator() {
  for (Rule* r : rules_) {
    delete r;
  }
  delete vars_;
  // for (auto p : rule_vars) {
  //   delete p.second;
  // }
}

void Evaluator::EvalAssign(const AssignAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  const char* origin = "file";

  StringPiece lhs = Intern(*ast->lhs->Eval(this));
  Var* rhs = NULL;
  bool needs_assign = true;
  switch (ast->op) {
    case AssignOp::COLON_EQ:
      rhs = new SimpleVar(ast->rhs->Eval(this), origin);
      break;
    case AssignOp::EQ:
      rhs = new RecursiveVar(ast->rhs, origin);
      break;
    case AssignOp::PLUS_EQ: {
      Var* prev = LookupVarInCurrentScope(lhs);
      if (!prev->IsDefined()) {
        rhs = new RecursiveVar(ast->rhs, origin);
      } else {
        prev->AppendVar(this, ast->rhs);
        rhs = prev;
        needs_assign = false;
      }
      break;
    }
    case AssignOp::QUESTION_EQ: {
      Var* prev = LookupVarInCurrentScope(lhs);
      if (!prev->IsDefined()) {
        rhs = new RecursiveVar(ast->rhs, origin);
      } else {
        // TODO
        abort();
      }
      break;
    }
  }

  LOG("Assign: %.*s=%s", SPF(lhs), rhs->DebugString().c_str());
  if (needs_assign)
    vars_->Assign(lhs, rhs);
}

void Evaluator::EvalRule(const RuleAST* ast) {
  loc_ = ast->loc();
  last_rule_ = NULL;

  shared_ptr<string> expr = ast->expr->Eval(this);
  // See semicolon.mk.
  if (expr->find_first_not_of(" \t\n;") == string::npos)
    return;

  Rule* rule = new Rule;
  rule->loc = loc_;
  rule->Parse(*expr);

  LOG("Rule: %s", rule->DebugString().c_str());
  rules_.push_back(rule);
  last_rule_ = rule;
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

void Evaluator::EvalInclude(const IncludeAST* ast) {
  ERROR("TODO");
}

void Evaluator::EvalExport(const ExportAST* ast) {
  ERROR("TODO");
}

Var* Evaluator::LookupVar(StringPiece name) {
  // TODO: TSV.
  Var* v = vars_->Lookup(name);
  if (v->IsDefined())
    return v;
  return in_vars_->Lookup(name);
}

Var* Evaluator::LookupVarInCurrentScope(StringPiece name) {
  // TODO: TSV.
  Var* v = vars_->Lookup(name);
  if (v->IsDefined())
    return v;
  return in_vars_->Lookup(name);
}

EvalResult* Evaluator::GetEvalResult() {
  EvalResult* er = new EvalResult;
  er->rules.swap(rules_);
  er->vars = vars_;
  vars_ = NULL;
  er->rule_vars.swap(rule_vars_);
  return er;
}

void Evaluator::Error(const string& msg) {
  ERROR("%s:%d: %s", LOCF(loc_), msg.c_str());
}
