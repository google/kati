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
#include <vector>

#include "loc.h"
#include "string_piece.h"
#include "symtab.h"

using namespace std;

class Evaluator;
class Value;

enum struct AssignOp {
  EQ,
  COLON_EQ,
  PLUS_EQ,
  QUESTION_EQ,
};

enum struct AssignDirective  {
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
  StringPiece orig() const { return orig_; }

  virtual void Eval(Evaluator* ev) const = 0;

  virtual string DebugString() const = 0;

 protected:
  Stmt();

 private:
  Loc loc_;
  StringPiece orig_;
};

struct RuleStmt : public Stmt {
  Value* expr;
  char term;
  Value* after_term;

  virtual ~RuleStmt();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct AssignStmt : public Stmt {
  Value* lhs;
  Value* rhs;
  StringPiece orig_rhs;
  AssignOp op;
  AssignDirective directive;

  AssignStmt()
      : lhs_sym_cache_(Symbol::IsUninitialized{}) {
  }
  virtual ~AssignStmt();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;

  Symbol GetLhsSymbol(Evaluator* ev) const;

 private:
  mutable Symbol lhs_sym_cache_;
};

struct CommandStmt : public Stmt {
  Value* expr;
  StringPiece orig;

  virtual ~CommandStmt();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct IfStmt : public Stmt {
  CondOp op;
  Value* lhs;
  Value* rhs;
  vector<Stmt*> true_stmts;
  vector<Stmt*> false_stmts;

  virtual ~IfStmt();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct IncludeStmt : public Stmt {
  Value* expr;
  bool should_exist;

  virtual ~IncludeStmt();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct ExportStmt : public Stmt {
  Value* expr;
  bool is_export;

  virtual ~ExportStmt();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct ParseErrorStmt : public Stmt {
  string msg;

  virtual ~ParseErrorStmt();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

#endif  // STMT_H_
