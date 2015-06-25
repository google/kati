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

#include "log.h"
#include "value.h"

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

Var::Var() {
}

Var::~Var() {
}

void Var::AppendVar(Evaluator*, Value*) {
  CHECK(false);
}

SimpleVar::SimpleVar(shared_ptr<string> v, VarOrigin origin)
    : v_(v), origin_(origin) {
}

void SimpleVar::Eval(Evaluator*, string* s) const {
  *s += *v_;
}

void SimpleVar::AppendVar(Evaluator* ev, Value* v) {
  shared_ptr<string> s = make_shared<string>(*v_);
  s->push_back(' ');
  v->Eval(ev, s.get());
  v_ = s;
}

StringPiece SimpleVar::String() const {
  return *v_;
}

string SimpleVar::DebugString() const {
  return *v_;
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
  return STRING_PIECE("");
}

string UndefinedVar::DebugString() const {
  return "*undefined*";
}

Vars::~Vars() {
  for (auto p : *this) {
    delete p.second;
  }
}

Var* Vars::Lookup(StringPiece name) const {
  auto found = find(name);
  if (found == end())
    return kUndefined;
  return found->second;
}

void Vars::Assign(StringPiece name, Var* v) {
  auto p = insert(make_pair(name, v));
  if (!p.second) {
    Var* orig = p.first->second;
    if (orig->Origin() == VarOrigin::OVERRIDE ||
        orig->Origin() == VarOrigin::ENVIRONMENT_OVERRIDE) {
      return;
    }
    if (orig->IsDefined())
      delete p.first->second;
    p.first->second = v;
  }
}

ScopedVar::ScopedVar(Vars* vars, StringPiece name, Var* var)
    : vars_(vars), orig_(NULL) {
  auto p = vars->insert(make_pair(name, var));
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
