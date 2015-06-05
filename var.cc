#include "var.h"

#include "value.h"

UndefinedVar kUndefinedBuf;
UndefinedVar* kUndefined = &kUndefinedBuf;

Var::Var() {
}

Var::~Var() {
}

SimpleVar::SimpleVar(shared_ptr<string> v, const char* origin)
    : v_(v), origin_(origin) {
}

void SimpleVar::Eval(Evaluator*, string* s) const {
  *s += *v_;
}

string SimpleVar::DebugString() const {
  return *v_;
}

RecursiveVar::RecursiveVar(Value* v, const char* origin)
    : v_(v), origin_(origin) {
}

void RecursiveVar::Eval(Evaluator* ev, string* s) const {
  v_->Eval(ev, s);
}

string RecursiveVar::DebugString() const {
  return v_->DebugString();
}

UndefinedVar::UndefinedVar() {}

void UndefinedVar::Eval(Evaluator*, string*) const {
  // Nothing to do.
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
    if (p.first->second->IsDefined())
      delete p.first->second;
    p.first->second = v;
  }
}
