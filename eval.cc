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
        // TODO
        abort();
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
