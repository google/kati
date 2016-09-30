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

#include "expr.h"
#include "log.h"

UndefinedVar kUndefinedBuf;
UndefinedVar* kUndefined = &kUndefinedBuf;

const char* GetOriginStr(VarOrigin origin) {
  switch (origin) {
    case VarOrigin::UNDEFINED: return "undefined";
    case VarOrigin::DEFAULT: return "default";
    case VarOrigin::ENVIRONMENT: return "environment";
    case VarOrigin::ENVIRONMENT_OVERRIDE: return "environment override";
    case VarOrigin::FILE: return "file";
    case VarOrigin::COMMAND_LINE: return "command line";
    case VarOrigin::OVERRIDE: return "override";
    case VarOrigin::AUTOMATIC: return "automatic";
  }
  CHECK(false);
  return "*** broken origin ***";
}

Var::Var() : readonly_(false) {
}

Var::~Var() {
}

void Var::AppendVar(Evaluator*, Value*) {
  CHECK(false);
}

SimpleVar::SimpleVar(VarOrigin origin)
    : origin_(origin) {
}

SimpleVar::SimpleVar(const string& v, VarOrigin origin)
    : v_(v), origin_(origin) {
}

void SimpleVar::Eval(Evaluator*, string* s) const {
  *s += v_;
}

void SimpleVar::AppendVar(Evaluator* ev, Value* v) {
  string buf;
  v->Eval(ev, &buf);
  v_.push_back(' ');
  v_ += buf;
}

StringPiece SimpleVar::String() const {
  return v_;
}

string SimpleVar::DebugString() const {
  return v_;
}

RecursiveVar::RecursiveVar(Value* v, VarOrigin origin, StringPiece orig)
    : v_(v), origin_(origin), orig_(orig) {
}

void RecursiveVar::Eval(Evaluator* ev, string* s) const {
  v_->Eval(ev, s);
}

void RecursiveVar::AppendVar(Evaluator*, Value* v) {
  v_ = NewExpr3(v_, NewLiteral(" "), v);
}

StringPiece RecursiveVar::String() const {
  return orig_;
}

string RecursiveVar::DebugString() const {
  return v_->DebugString();
}

UndefinedVar::UndefinedVar() {}

void UndefinedVar::Eval(Evaluator*, string*) const {
  // Nothing to do.
}

StringPiece UndefinedVar::String() const {
  return StringPiece("");
}

string UndefinedVar::DebugString() const {
  return "*undefined*";
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
    return kUndefined;
  Var* v = found->second;
  if (v->Origin() == VarOrigin::ENVIRONMENT ||
      v->Origin() == VarOrigin::ENVIRONMENT_OVERRIDE) {
    used_env_vars_.insert(name);
  }
  return v;
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

unordered_set<Symbol> Vars::used_env_vars_;

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
