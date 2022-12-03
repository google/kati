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

#include "stmt.h"

#include "eval.h"
#include "expr.h"
#include "stringprintf.h"
#include "strutil.h"

Stmt::Stmt() {}

Stmt::~Stmt() {}

void Stmt::Eval(Evaluator* ev) const {
  ScopedFrame frame(ev->Enter(FrameType::STATEMENT, "statement", loc_));
  EvalStatement(ev);
}

std::string RuleStmt::DebugString() const {
  return StringPrintf("RuleStmt(lhs=%s sep=%d rhs=%s loc=%s:%d)",
                      Value::DebugString(lhs).c_str(), sep,
                      Value::DebugString(rhs).c_str(), LOCF(loc()));
}

std::string AssignStmt::DebugString() const {
  const char* opstr = "???";
  switch (op) {
    case AssignOp::EQ:
      opstr = "EQ";
      break;
    case AssignOp::COLON_EQ:
      opstr = "COLON_EQ";
      break;
    case AssignOp::PLUS_EQ:
      opstr = "PLUS_EQ";
      break;
    case AssignOp::QUESTION_EQ:
      opstr = "QUESTION_EQ";
      break;
  }
  const char* dirstr = "???";
  switch (directive) {
    case AssignDirective::NONE:
      dirstr = "";
      break;
    case AssignDirective::OVERRIDE:
      dirstr = "override";
      break;
    case AssignDirective::EXPORT:
      dirstr = "export";
      break;
  }
  return StringPrintf(
      "AssignStmt(lhs=%s rhs=%s (%s) "
      "opstr=%s dir=%s loc=%s:%d)",
      Value::DebugString(lhs).c_str(), Value::DebugString(rhs).c_str(),
      NoLineBreak(std::string(orig_rhs)).c_str(), opstr, dirstr, LOCF(loc()));
}

Symbol AssignStmt::GetLhsSymbol(Evaluator* ev) const {
  if (!lhs->IsLiteral()) {
    std::string buf;
    lhs->Eval(ev, &buf);
    return Intern(buf);
  }

  if (!lhs_sym_cache_.IsValid()) {
    lhs_sym_cache_ = Intern(lhs->GetLiteralValueUnsafe());
  }
  return lhs_sym_cache_;
}

std::string CommandStmt::DebugString() const {
  return StringPrintf("CommandStmt(%s, loc=%s:%d)",
                      Value::DebugString(expr).c_str(), LOCF(loc()));
}

std::string IfStmt::DebugString() const {
  const char* opstr = "???";
  switch (op) {
    case CondOp::IFEQ:
      opstr = "ifeq";
      break;
    case CondOp::IFNEQ:
      opstr = "ifneq";
      break;
    case CondOp::IFDEF:
      opstr = "ifdef";
      break;
    case CondOp::IFNDEF:
      opstr = "ifndef";
      break;
  }
  return StringPrintf("IfStmt(op=%s, lhs=%s, rhs=%s t=%zu f=%zu loc=%s:%d)",
                      opstr, Value::DebugString(lhs).c_str(),
                      Value::DebugString(rhs).c_str(), true_stmts.size(),
                      false_stmts.size(), LOCF(loc()));
}

std::string IncludeStmt::DebugString() const {
  return StringPrintf("IncludeStmt(%s, loc=%s:%d)",
                      Value::DebugString(expr).c_str(), LOCF(loc()));
}

std::string ExportStmt::DebugString() const {
  return StringPrintf("ExportStmt(%s, %d, loc=%s:%d)",
                      Value::DebugString(expr).c_str(), is_export, LOCF(loc()));
}

std::string ParseErrorStmt::DebugString() const {
  return StringPrintf("ParseErrorStmt(%s, loc=%s:%d)", msg.c_str(),
                      LOCF(loc()));
}

RuleStmt::~RuleStmt() {
  delete lhs;
  delete rhs;
}

void RuleStmt::EvalStatement(Evaluator* ev) const {
  ev->EvalRule(this);
}

AssignStmt::~AssignStmt() {
  delete lhs;
  delete rhs;
}

void AssignStmt::EvalStatement(Evaluator* ev) const {
  ev->EvalAssign(this);
}

CommandStmt::~CommandStmt() {
  delete expr;
}

void CommandStmt::EvalStatement(Evaluator* ev) const {
  ev->EvalCommand(this);
}

IfStmt::~IfStmt() {
  delete lhs;
  delete rhs;
}

void IfStmt::EvalStatement(Evaluator* ev) const {
  ev->EvalIf(this);
}

IncludeStmt::~IncludeStmt() {
  delete expr;
}

void IncludeStmt::EvalStatement(Evaluator* ev) const {
  ev->EvalInclude(this);
}

ExportStmt::~ExportStmt() {
  delete expr;
}

void ExportStmt::EvalStatement(Evaluator* ev) const {
  ev->EvalExport(this);
}

ParseErrorStmt::~ParseErrorStmt() {}

void ParseErrorStmt::EvalStatement(Evaluator* ev) const {
  ev->set_loc(loc());
  ev->Error(msg);
}
