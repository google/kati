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

#ifndef AST_H_
#define AST_H_

#include <string>
#include <vector>

#include "loc.h"
#include "string_piece.h"

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

struct AST {
 public:
  virtual ~AST();

  Loc loc() const { return loc_; }
  void set_loc(Loc loc) { loc_ = loc; }
  StringPiece orig() const { return orig_; }

  virtual void Eval(Evaluator* ev) const = 0;

  virtual string DebugString() const = 0;

 protected:
  AST();

 private:
  Loc loc_;
  StringPiece orig_;
};

struct RuleAST : public AST {
  Value* expr;
  char term;
  Value* after_term;

  virtual ~RuleAST();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct AssignAST : public AST {
  Value* lhs;
  Value* rhs;
  StringPiece orig_rhs;
  AssignOp op;
  AssignDirective directive;

  virtual ~AssignAST();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct CommandAST : public AST {
  Value* expr;
  StringPiece orig;

  virtual ~CommandAST();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct IfAST : public AST {
  CondOp op;
  Value* lhs;
  Value* rhs;
  vector<AST*> true_asts;
  vector<AST*> false_asts;

  virtual ~IfAST();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct IncludeAST : public AST {
  Value* expr;
  bool should_exist;

  virtual ~IncludeAST();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

struct ExportAST : public AST {
  Value* expr;
  bool is_export;

  virtual ~ExportAST();

  virtual void Eval(Evaluator* ev) const;

  virtual string DebugString() const;
};

#endif  // AST_H_
