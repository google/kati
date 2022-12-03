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

#include "var.h"

#include "eval.h"
#include "expr.h"
#include "log.h"
#include "strutil.h"

std::unordered_map<const Var*, std::string> Var::diagnostic_messages_;

const char* GetOriginStr(VarOrigin origin) {
  switch (origin) {
    case VarOrigin::UNDEFINED:
      return "undefined";
    case VarOrigin::DEFAULT:
      return "default";
    case VarOrigin::ENVIRONMENT:
      return "environment";
    case VarOrigin::ENVIRONMENT_OVERRIDE:
      return "environment override";
    case VarOrigin::FILE:
      return "file";
    case VarOrigin::COMMAND_LINE:
      return "command line";
    case VarOrigin::OVERRIDE:
      return "override";
    case VarOrigin::AUTOMATIC:
      return "automatic";
  }
  CHECK(false);
  return "*** broken origin ***";
}

Var::Var() : Var(VarOrigin::UNDEFINED, nullptr, Loc()) {}

Var::Var(VarOrigin origin, Frame* definition, Loc loc)
    : Evaluable(loc),
      definition_(definition),
      origin_(origin),
      readonly_(false),
      deprecated_(false),
      obsolete_(false),
      self_referential_(false) {}

Var::~Var() {
  diagnostic_messages_.erase(this);
}

void Var::AppendVar(Evaluator*, Value*) {
  CHECK(false);
}

void Var::SetDeprecated(const std::string_view& msg) {
  deprecated_ = true;
  diagnostic_messages_[this] = std::string(msg);
}

void Var::SetObsolete(const std::string_view& msg) {
  obsolete_ = true;
  diagnostic_messages_[this] = std::string(msg);
}

void Var::Used(Evaluator* ev, const Symbol& sym) const {
  if (obsolete_) {
    ev->Error(StringPrintf("*** %s is obsolete%s.", sym.c_str(),
                           diagnostic_message_text()));
  } else if (deprecated_) {
    WARN_LOC(ev->loc(), "%s has been deprecated%s.", sym.c_str(),
             diagnostic_message_text());
  }
}

const char* Var::diagnostic_message_text() const {
  auto it = diagnostic_messages_.find(this);
  return it == diagnostic_messages_.end() ? "" : it->second.c_str();
}

const std::string& Var::DeprecatedMessage() const {
  static const std::string empty_string;
  auto it = diagnostic_messages_.find(this);
  return it == diagnostic_messages_.end() ? empty_string : it->second;
}

Var* Var::Undefined() {
  static Var* undefined_var;
  if (!undefined_var) {
    undefined_var = new UndefinedVar();
  }
  return undefined_var;
}

SimpleVar::SimpleVar(VarOrigin origin, Frame* definition, Loc loc)
    : Var(origin, definition, loc) {}

SimpleVar::SimpleVar(const std::string& v,
                     VarOrigin origin,
                     Frame* definition,
                     Loc loc)
    : Var(origin, definition, loc), v_(v) {}

SimpleVar::SimpleVar(VarOrigin origin,
                     Frame* definition,
                     Loc loc,
                     Evaluator* ev,
                     Value* v)
    : Var(origin, definition, loc) {
  v->Eval(ev, &v_);
}

bool SimpleVar::IsFunc(Evaluator*) const {
  return false;
}

void SimpleVar::Eval(Evaluator* ev, std::string* s) const {
  ev->CheckStack();
  *s += v_;
}

void SimpleVar::AppendVar(Evaluator* ev, Value* v) {
  std::string buf;
  v->Eval(ev, &buf);
  v_.push_back(' ');
  v_ += buf;
  definition_ = ev->CurrentFrame();
}

std::string_view SimpleVar::String() const {
  return v_;
}

std::string SimpleVar::DebugString() const {
  return v_;
}

RecursiveVar::RecursiveVar(Value* v,
                           VarOrigin origin,
                           Frame* definition,
                           Loc loc,
                           std::string_view orig)
    : Var(origin, definition, loc), v_(v), orig_(orig) {}

bool RecursiveVar::IsFunc(Evaluator* ev) const {
  return v_->IsFunc(ev);
}

void RecursiveVar::Eval(Evaluator* ev, std::string* s) const {
  ev->CheckStack();
  v_->Eval(ev, s);
}

void RecursiveVar::AppendVar(Evaluator* ev, Value* v) {
  ev->CheckStack();
  v_ = Value::NewExpr(v->Location(), v_, Value::NewLiteral(" "), v);
  definition_ = ev->CurrentFrame();
}

void RecursiveVar::Used(Evaluator* ev, const Symbol& sym) const {
  if (SelfReferential()) {
    ERROR_LOC(
        Location(),
        StringPrintf(
            "*** Recursive variable \"%s\" references itself (eventually).",
            sym.c_str())
            .c_str());
  }

  Var::Used(ev, sym);
}

std::string_view RecursiveVar::String() const {
  return orig_;
}

std::string RecursiveVar::DebugString() const {
  return Value::DebugString(v_);
}

UndefinedVar::UndefinedVar() {}

bool UndefinedVar::IsFunc(Evaluator*) const {
  return false;
}

void UndefinedVar::Eval(Evaluator*, std::string*) const {
  // Nothing to do.
}

std::string_view UndefinedVar::String() const {
  return std::string_view("");
}

std::string UndefinedVar::DebugString() const {
  return "*undefined*";
}

VariableNamesVar::VariableNamesVar(std::string_view name, bool all)
    : name_(name), all_(all) {
  SetReadOnly();
  SetAssignOp(AssignOp::COLON_EQ);
}

bool VariableNamesVar::IsFunc(Evaluator*) const {
  return false;
}

void VariableNamesVar::Eval(Evaluator* ev, std::string* s) const {
  ConcatVariableNames(ev, s);
}

std::string_view VariableNamesVar::String() const {
  return name_;
}

void VariableNamesVar::ConcatVariableNames(Evaluator* ev,
                                           std::string* s) const {
  WordWriter ww(s);
  std::vector<std::string_view>&& symbols =
      GetSymbolNames([=](Var* var) -> bool {
        if (var->Obsolete()) {
          return false;
        }
        if (!all_) {
          if (var->IsFunc(ev)) {
            return false;
          }
        }
        return true;
      });
  for (auto entry : symbols) {
    ww.Write(entry);
  }
}

std::string VariableNamesVar::DebugString() const {
  return "*VariableNamesVar*";
}

bool ShellStatusVar::is_set_ = false;
int ShellStatusVar::shell_status_ = 0;
std::string ShellStatusVar::shell_status_string_;

ShellStatusVar::ShellStatusVar() {
  SetReadOnly();
  SetAssignOp(AssignOp::COLON_EQ);
}

void ShellStatusVar::SetValue(int newShellStatus) {
  if (!is_set_ || shell_status_ != newShellStatus) {
    shell_status_ = newShellStatus;
    is_set_ = true;
    shell_status_string_.clear();
  }
}

bool ShellStatusVar::IsDefined() const {
  return is_set_;
}

bool ShellStatusVar::IsFunc(Evaluator*) const {
  return false;
}

void ShellStatusVar::Eval(Evaluator* ev, std::string* s) const {
  if (ev->IsEvaluatingCommand()) {
    ev->Error("Kati does not support using .SHELLSTATUS inside of a rule");
  }

  if (!is_set_) {
    return;
  }

  *s += String();
}

std::string_view ShellStatusVar::String() const {
  if (!is_set_) {
    return "";
  }

  if (shell_status_string_.empty()) {
    shell_status_string_ = std::to_string(shell_status_);
  }

  return shell_status_string_;
}

std::string ShellStatusVar::DebugString() const {
  return "*ShellStatusVar*";
}

Vars::~Vars() {
  for (auto p : *this) {
    delete p.second;
  }
}

void Vars::add_used_env_vars(Symbol v) {
  used_env_vars_.insert(v);
}

Var* Vars::Lookup(Symbol name) const {
  auto found = find(name);
  if (found == end())
    return Var::Undefined();
  Var* v = found->second;
  if (v->Origin() == VarOrigin::ENVIRONMENT ||
      v->Origin() == VarOrigin::ENVIRONMENT_OVERRIDE) {
    used_env_vars_.insert(name);
  }
  return v;
}

Var* Vars::Peek(Symbol name) const {
  auto found = find(name);
  return found == end() ? Var::Undefined() : found->second;
}

void Vars::Assign(Symbol name, Var* v, bool* readonly) {
  *readonly = false;
  auto p = emplace(name, v);
  if (!p.second) {
    Var* orig = p.first->second;
    if (orig->ReadOnly()) {
      *readonly = true;
      return;
    }
    if (orig->Origin() == VarOrigin::OVERRIDE ||
        orig->Origin() == VarOrigin::ENVIRONMENT_OVERRIDE) {
      return;
    }
    if (orig->Origin() == VarOrigin::AUTOMATIC) {
      ERROR("overriding automatic variable is not implemented yet");
    }
    if (orig->IsDefined())
      delete p.first->second;
    p.first->second = v;
  }
}

SymbolSet Vars::used_env_vars_;

ScopedVar::ScopedVar(Vars* vars, Symbol name, Var* var)
    : vars_(vars), orig_(NULL) {
  auto p = vars->emplace(name, var);
  iter_ = p.first;
  if (!p.second) {
    orig_ = iter_->second;
    iter_->second = var;
  }
}

ScopedVar::~ScopedVar() {
  if (orig_) {
    iter_->second = orig_;
  } else {
    vars_->erase(iter_);
  }
}
