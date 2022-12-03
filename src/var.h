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

#ifndef VAR_H_
#define VAR_H_

#include <memory>
#include <string>
#include <string_view>
#include <unordered_map>
#include <unordered_set>

#include "eval.h"
#include "expr.h"
#include "loc.h"
#include "log.h"
#include "stmt.h"
#include "symtab.h"

class Evaluator;
class Value;

enum struct VarOrigin : char {
  UNDEFINED,
  DEFAULT,
  ENVIRONMENT,
  ENVIRONMENT_OVERRIDE,
  FILE,
  COMMAND_LINE,
  OVERRIDE,
  AUTOMATIC,
};

const char* GetOriginStr(VarOrigin origin);

class Var : public Evaluable {
 public:
  virtual ~Var();

  virtual const char* Flavor() const = 0;

  VarOrigin Origin() const { return origin_; }
  Frame* Definition() const { return definition_; }

  virtual bool IsDefined() const { return true; }

  virtual void AppendVar(Evaluator* ev, Value* v);

  virtual std::string_view String() const = 0;

  virtual std::string DebugString() const = 0;

  bool ReadOnly() const { return readonly_; }
  void SetReadOnly() { readonly_ = true; }

  bool Deprecated() const { return deprecated_; }
  void SetDeprecated(const std::string_view& msg);

  bool Obsolete() const { return obsolete_; }
  void SetObsolete(const std::string_view& msg);

  bool SelfReferential() const { return self_referential_; }
  void SetSelfReferential() { self_referential_ = true; }

  const std::string& DeprecatedMessage() const;

  // This variable was used (either written or read from)
  virtual void Used(Evaluator* ev, const Symbol& sym) const;

  AssignOp op() const { return assign_op_; }
  void SetAssignOp(AssignOp op) { assign_op_ = op; }

  static Var* Undefined();

 protected:
  Var();
  Var(VarOrigin origin, Frame* definition, Loc loc);

  Frame* definition_;

 private:
  const VarOrigin origin_;

  AssignOp assign_op_;
  bool readonly_ : 1;
  bool deprecated_ : 1;
  bool obsolete_ : 1;
  bool self_referential_ : 1;

  const char* diagnostic_message_text() const;

  static std::unordered_map<const Var*, std::string> diagnostic_messages_;
};

class SimpleVar : public Var {
 public:
  explicit SimpleVar(VarOrigin origin, Frame* definition, Loc loc);
  SimpleVar(const std::string& v, VarOrigin origin, Frame* definition, Loc loc);
  SimpleVar(VarOrigin origin,
            Frame* definition,
            Loc loc,
            Evaluator* ev,
            Value* v);

  virtual const char* Flavor() const override { return "simple"; }

  virtual bool IsFunc(Evaluator* ev) const override;

  virtual void Eval(Evaluator* ev, std::string* s) const override;

  virtual void AppendVar(Evaluator* ev, Value* v) override;

  virtual std::string_view String() const override;

  virtual std::string DebugString() const override;

  std::string v_;
};

class RecursiveVar : public Var {
 public:
  RecursiveVar(Value* v,
               VarOrigin origin,
               Frame* definition,
               Loc loc,
               std::string_view orig);

  virtual const char* Flavor() const override { return "recursive"; }

  virtual bool IsFunc(Evaluator* ev) const override;

  virtual void Eval(Evaluator* ev, std::string* s) const override;

  virtual void AppendVar(Evaluator* ev, Value* v) override;

  virtual std::string_view String() const override;

  virtual std::string DebugString() const override;

  virtual void Used(Evaluator* ev, const Symbol& sym) const override;

  Value* v_;
  std::string_view orig_;
};

class UndefinedVar : public Var {
 public:
  UndefinedVar();

  virtual const char* Flavor() const override { return "undefined"; }
  virtual bool IsDefined() const override { return false; }

  virtual bool IsFunc(Evaluator* ev) const override;

  virtual void Eval(Evaluator* ev, std::string* s) const override;

  virtual std::string_view String() const override;

  virtual std::string DebugString() const override;
};

// The built-in VARIABLES and KATI_SYMBOLS variables
class VariableNamesVar : public Var {
 public:
  VariableNamesVar(std::string_view name, bool all);

  virtual const char* Flavor() const override { return "kati_variable_names"; }
  virtual bool IsDefined() const override { return true; }

  virtual bool IsFunc(Evaluator* ev) const override;

  virtual void Eval(Evaluator* ev, std::string* s) const override;

  virtual std::string_view String() const override;

  virtual std::string DebugString() const override;

 private:
  std::string_view name_;
  bool all_;

  void ConcatVariableNames(Evaluator* ev, std::string* s) const;
};

// The built-in .SHELLSTATUS variable
class ShellStatusVar : public Var {
 public:
  ShellStatusVar();

  static void SetValue(int newShellStatus);

  virtual const char* Flavor() const override { return "simple"; }
  virtual bool IsDefined() const override;

  virtual bool IsFunc(Evaluator* ev) const override;

  virtual void Eval(Evaluator* ev, std::string* s) const override;

  virtual std::string_view String() const override;

  virtual std::string DebugString() const override;

 private:
  static bool is_set_;
  static int shell_status_;
  static std::string shell_status_string_;
};

class Vars : public std::unordered_map<Symbol, Var*> {
 public:
  ~Vars();

  Var* Lookup(Symbol name) const;
  Var* Peek(Symbol name) const;

  void Assign(Symbol name, Var* v, bool* readonly);

  static void add_used_env_vars(Symbol v);

  static const SymbolSet used_env_vars() { return used_env_vars_; }

 private:
  static SymbolSet used_env_vars_;
};

class ScopedVar {
 public:
  // Does not take ownerships of arguments.
  ScopedVar(Vars* vars, Symbol name, Var* var);
  ~ScopedVar();

 private:
  Vars* vars_;
  Var* orig_;
  Vars::iterator iter_;
};

#endif  // VAR_H_
