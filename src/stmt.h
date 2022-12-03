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

#ifndef STMT_H_
#define STMT_H_

#include <string>
#include <string_view>
#include <vector>

#include "loc.h"
#include "symtab.h"

class Evaluator;
class Value;

enum struct AssignOp : char {
  EQ,
  COLON_EQ,
  PLUS_EQ,
  QUESTION_EQ,
};

enum struct AssignDirective {
  NONE = 0,
  OVERRIDE = 1,
  EXPORT = 2,
};

enum struct CondOp {
  IFEQ,
  IFNEQ,
  IFDEF,
  IFNDEF,
};

struct Stmt {
 public:
  virtual ~Stmt();

  Loc loc() const { return loc_; }
  void set_loc(Loc loc) { loc_ = loc; }
  std::string_view orig() const { return orig_; }

  void Eval(Evaluator*) const;
  virtual void EvalStatement(Evaluator* ev) const = 0;

  virtual std::string DebugString() const = 0;

 protected:
  Stmt();

 private:
  Loc loc_;
  std::string_view orig_;
};

/* Parsed "rule statement" before evaluation is kept as
 *    <lhs> <sep> <rhs>
 * where <lhs> and <rhs> as Value instances. <sep> is either command
 * separator (';') or an assignment ('=' or '=$=').
 * Until we evaluate <lhs>, we don't know whether it is a rule or
 * a rule-specific variable assignment.
 */
struct RuleStmt : public Stmt {
  Value* lhs;
  enum { SEP_NULL, SEP_SEMICOLON, SEP_EQ, SEP_FINALEQ } sep;
  Value* rhs;

  virtual ~RuleStmt();

  virtual void EvalStatement(Evaluator* ev) const;

  virtual std::string DebugString() const;
};

struct AssignStmt : public Stmt {
  Value* lhs;
  Value* rhs;
  std::string_view orig_rhs;
  AssignOp op;
  AssignDirective directive;
  bool is_final;

  AssignStmt() : is_final(false) {}
  virtual ~AssignStmt();

  virtual void EvalStatement(Evaluator* ev) const;

  virtual std::string DebugString() const;

  Symbol GetLhsSymbol(Evaluator* ev) const;

 private:
  mutable Symbol lhs_sym_cache_;
};

struct CommandStmt : public Stmt {
  Value* expr;
  std::string_view orig;

  virtual ~CommandStmt();

  virtual void EvalStatement(Evaluator* ev) const;

  virtual std::string DebugString() const;
};

struct IfStmt : public Stmt {
  CondOp op;
  Value* lhs;
  Value* rhs;
  std::vector<Stmt*> true_stmts;
  std::vector<Stmt*> false_stmts;

  virtual ~IfStmt();

  virtual void EvalStatement(Evaluator* ev) const;

  virtual std::string DebugString() const;
};

struct IncludeStmt : public Stmt {
  Value* expr;
  bool should_exist;

  virtual ~IncludeStmt();

  virtual void EvalStatement(Evaluator* ev) const;

  virtual std::string DebugString() const;
};

struct ExportStmt : public Stmt {
  Value* expr;
  bool is_export;

  virtual ~ExportStmt();

  virtual void EvalStatement(Evaluator* ev) const;

  virtual std::string DebugString() const;
};

struct ParseErrorStmt : public Stmt {
  std::string msg;

  virtual ~ParseErrorStmt();

  virtual void EvalStatement(Evaluator* ev) const;

  virtual std::string DebugString() const;
};

#endif  // STMT_H_
