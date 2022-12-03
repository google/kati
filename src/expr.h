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

#ifndef EXPR_H_
#define EXPR_H_

#include <string>
#include <string_view>
#include <vector>

#include "loc.h"

class Evaluator;

class Evaluable {
 public:
  virtual void Eval(Evaluator* ev, std::string* s) const = 0;
  std::string Eval(Evaluator*) const;
  const Loc& Location() const { return loc_; }
  // Whether this Evaluable is either knowably a function (e.g. one of the
  // built-ins) or likely to be a function-type macro (i.e. one that has
  // positional $(1) arguments to be expanded inside it. However, this is
  // only a heuristic guess. In order to not actually evaluate the expression,
  // because doing so could have side effects like calling $(error ...) or
  // doing a nested eval that assigns variables, we don't handle the case where
  // the variable name is itself a variable expansion inside a deferred
  // expansion variable, and return true in that case. Implementations of this
  // function must also not mark variables as used, as that can trigger unwanted
  // warnings. They should use ev->PeekVar().
  virtual bool IsFunc(Evaluator* ev) const = 0;

 protected:
  Evaluable(const Loc& loc);
  virtual ~Evaluable();

 private:
  const Loc loc_;
};

class Value : public Evaluable {
 public:
  // All NewExpr calls take ownership of the Value instances.
  static Value* NewExpr(const Loc& loc, Value* v1, Value* v2);
  static Value* NewExpr(const Loc& loc, Value* v1, Value* v2, Value* v3);
  static Value* NewExpr(const Loc& loc, std::vector<Value*>* values);

  static Value* NewLiteral(std::string_view s);
  virtual ~Value();
  virtual bool IsLiteral() const { return false; }
  // Only safe after IsLiteral() returns true.
  virtual std::string_view GetLiteralValueUnsafe() const { return ""; }

  static std::string DebugString(const Value*);

 protected:
  Value(const Loc& loc);
  virtual std::string DebugString_() const = 0;
};

enum struct ParseExprOpt {
  NORMAL = 0,
  DEFINE,
  COMMAND,
  FUNC,
};

Value* ParseExprImpl(Loc* loc,
                     std::string_view s,
                     const char* terms,
                     ParseExprOpt opt,
                     size_t* index_out,
                     bool trim_right_space = false);
Value* ParseExpr(Loc* loc,
                 std::string_view s,
                 ParseExprOpt opt = ParseExprOpt::NORMAL);

std::string JoinValues(const std::vector<Value*>& vals, const char* sep);

#endif  // EXPR_H_
