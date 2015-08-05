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

#include "ast.h"

#include "eval.h"
#include "stringprintf.h"
#include "strutil.h"
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
  return StringPrintf("AssignAST(lhs=%s rhs=%s (%s) "
                      "opstr=%s dir=%s loc=%s:%d)",
                      lhs->DebugString().c_str(),
                      rhs->DebugString().c_str(),
                      NoLineBreak(orig_rhs.as_string()).c_str(),
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
  return StringPrintf("IfAST(op=%s, lhs=%s, rhs=%s t=%zu f=%zu loc=%s:%d)",
                      opstr,
                      lhs->DebugString().c_str(),
                      rhs->DebugString().c_str(),
                      true_asts.size(),
                      false_asts.size(),
                      LOCF(loc()));
}

string IncludeAST::DebugString() const {
  return StringPrintf("IncludeAST(%s, loc=%s:%d)",
                      expr->DebugString().c_str(), LOCF(loc()));
}

string ExportAST::DebugString() const {
  return StringPrintf("ExportAST(%s, %d, loc=%s:%d)",
                      expr->DebugString().c_str(),
                      is_export,
                      LOCF(loc()));
}

string ParseErrorAST::DebugString() const {
  return StringPrintf("ParseErrorAST(%s, loc=%s:%d)",
                      msg.c_str(),
                      LOCF(loc()));
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

ExportAST::~ExportAST() {
  delete expr;
}

void ExportAST::Eval(Evaluator* ev) const {
  ev->EvalExport(this);
}

ParseErrorAST::~ParseErrorAST() {
}

void ParseErrorAST::Eval(Evaluator* ev) const {
  ev->set_loc(loc());
  ev->Error(msg);
}
