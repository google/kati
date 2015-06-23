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
#include <unordered_map>

#include "ast.h"
#include "string_piece.h"
#include "value.h"

using namespace std;

class Evaluator;
class Value;

class Var : public Evaluable {
 public:
  virtual ~Var();

  virtual const char* Flavor() const = 0;
  virtual const char* Origin() const = 0;
  virtual bool IsDefined() const { return true; }

  virtual void AppendVar(Evaluator* ev, Value* v);

  virtual StringPiece String() const = 0;

  virtual string DebugString() const = 0;

 protected:
  Var();
};

class SimpleVar : public Var {
 public:
  SimpleVar(shared_ptr<string> v, const char* origin);

  virtual const char* Flavor() const {
    return "simple";
  }
  virtual const char* Origin() const {
    return origin_;
  }

  virtual shared_ptr<string> Eval(Evaluator*) const override {
    return v_;
  }
  virtual void Eval(Evaluator* ev, string* s) const override;

  virtual void AppendVar(Evaluator* ev, Value* v);

  virtual StringPiece String() const override;

  virtual string DebugString() const override;

 private:
  shared_ptr<string> v_;
  const char* origin_;
};

class RecursiveVar : public Var {
 public:
  RecursiveVar(Value* v, const char* origin, StringPiece orig);

  virtual const char* Flavor() const {
    return "recursive";
  }
  virtual const char* Origin() const {
    return origin_;
  }

  virtual void Eval(Evaluator* ev, string* s) const override;

  virtual void AppendVar(Evaluator* ev, Value* v);

  virtual StringPiece String() const override;

  virtual string DebugString() const override;

 private:
  Value* v_;
  const char* origin_;
  StringPiece orig_;
};

class UndefinedVar : public Var {
 public:
  UndefinedVar();

  virtual const char* Flavor() const {
    return "undefined";
  }
  virtual const char* Origin() const {
    return "undefined";
  }
  virtual bool IsDefined() const { return false; }

  virtual void Eval(Evaluator* ev, string* s) const override;

  virtual StringPiece String() const override;

  virtual string DebugString() const override;
};

extern UndefinedVar* kUndefined;

class RuleVar : public Var {
 public:
  RuleVar(Var* v, AssignOp op)
      : v_(v), op_(op) {}
  virtual ~RuleVar() {
    delete v_;
  }

  virtual const char* Flavor() const {
    return v_->Flavor();
  }
  virtual const char* Origin() const {
    return v_->Origin();
  }
  virtual bool IsDefined() const {
    return v_->IsDefined();
  }
  virtual void Eval(Evaluator* ev, string* s) const {
    v_->Eval(ev, s);
  }
  virtual void AppendVar(Evaluator* ev, Value* v) {
    v_->AppendVar(ev, v);
  }
  virtual StringPiece String() const override {
    return v_->String();
  }
  virtual string DebugString() const override {
    return v_->DebugString();
  }

  Var* v() const { return v_; }
  AssignOp op() const { return op_; }

 private:
  Var* v_;
  AssignOp op_;
};

class Vars : public unordered_map<StringPiece, Var*> {
 public:
  ~Vars();

  Var* Lookup(StringPiece name) const;

  void Assign(StringPiece name, Var* v);
};

class ScopedVar {
 public:
  // Does not take ownerships of arguments.
  ScopedVar(Vars* vars, StringPiece name, Var* var);
  ~ScopedVar();

 private:
  Vars* vars_;
  Var* orig_;
  Vars::iterator iter_;
};

#endif  // VAR_H_
