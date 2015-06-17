#include "ast.h"

#include "eval.h"
#include "stringprintf.h"
#include "value.h"

AST::AST() {}

AST::~AST() {}

string RuleAST::DebugString() const {
  return StringPrintf("RuleAST(expr=%s term=%d after_term=%s loc=%s:%d)",
                      expr->DebugString().c_str(),
                      term,
                      after_term->DebugString().c_str(),
                      LOCF(loc()));
}

string AssignAST::DebugString() const {
  const char* opstr = "???";
  switch (op) {
    case AssignOp::EQ: opstr = "EQ"; break;
    case AssignOp::COLON_EQ: opstr = "COLON_EQ"; break;
    case AssignOp::PLUS_EQ: opstr = "PLUS_EQ"; break;
    case AssignOp::QUESTION_EQ: opstr = "QUESTION_EQ"; break;
  }
  const char* dirstr = "???";
  switch (directive) {
    case AssignDirective::NONE: dirstr = ""; break;
    case AssignDirective::OVERRIDE: dirstr = "override"; break;
    case AssignDirective::EXPORT: dirstr = "export"; break;
  }
  return StringPrintf("AssignAST(lhs=%s rhs=%s opstr=%s dir=%s loc=%s:%d)",
                      lhs->DebugString().c_str(),
                      rhs->DebugString().c_str(),
                      opstr, dirstr, LOCF(loc()));
}

string CommandAST::DebugString() const {
  return StringPrintf("CommandAST(%s, loc=%s:%d)",
                      expr->DebugString().c_str(), LOCF(loc()));
}

string IfAST::DebugString() const {
  const char* opstr = "???";
  switch (op) {
    case CondOp::IFEQ: opstr = "ifeq"; break;
    case CondOp::IFNEQ: opstr = "ifneq"; break;
    case CondOp::IFDEF: opstr = "ifdef"; break;
    case CondOp::IFNDEF: opstr = "ifndef"; break;
  }
  return StringPrintf("IfAST(op=%s, lhs=%s, rhs=%s loc=%s:%d)",
                      opstr,
                      lhs->DebugString().c_str(),
                      rhs->DebugString().c_str(),
                      LOCF(loc()));
}

string IncludeAST::DebugString() const {
  return StringPrintf("IncludeAST(%s, loc=%s:%d)",
                      expr->DebugString().c_str(), LOCF(loc()));
}

RuleAST::~RuleAST() {
  delete expr;
  delete after_term;
}

void RuleAST::Eval(Evaluator* ev) const {
  ev->EvalRule(this);
}

AssignAST::~AssignAST() {
  delete lhs;
  delete rhs;
}

void AssignAST::Eval(Evaluator* ev) const {
  ev->EvalAssign(this);
}

CommandAST::~CommandAST() {
  delete expr;
}

void CommandAST::Eval(Evaluator* ev) const {
  ev->EvalCommand(this);
}

IfAST::~IfAST() {
  delete lhs;
  delete rhs;
}

void IfAST::Eval(Evaluator* ev) const {
  ev->EvalIf(this);
}

IncludeAST::~IncludeAST() {
  delete expr;
}

void IncludeAST::Eval(Evaluator* ev) const {
  ev->EvalInclude(this);
}
