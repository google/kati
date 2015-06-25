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

#include "value.h"

#include <vector>

#include "eval.h"
#include "func.h"
#include "log.h"
#include "stringprintf.h"
#include "strutil.h"
#include "var.h"

Evaluable::Evaluable() {
}

Evaluable::~Evaluable() {
}

shared_ptr<string> Evaluable::Eval(Evaluator* ev) const {
  shared_ptr<string> s = make_shared<string>();
  Eval(ev, s.get());
  return s;
}

Value::Value() {
}

Value::~Value() {
}

string Value::DebugString() const {
  if (this) {
    return NoLineBreak(DebugString_());
  }
  return "(null)";
}

class Literal : public Value {
 public:
  explicit Literal(StringPiece s)
      : s_(s) {
  }

  StringPiece val() const { return s_; }

  virtual void Eval(Evaluator*, string* s) const override {
    s->append(s_.begin(), s_.end());
  }

  virtual string DebugString_() const override {
    return s_.as_string();
  }

 private:
  StringPiece s_;
};

class Expr : public Value {
 public:
  Expr() {
  }

  virtual ~Expr() {
    for (Value* v : vals_) {
      delete v;
    }
  }

  // Takes the ownership of |v|.
  void AddValue(Value* v) {
    vals_.push_back(v);
  }

  virtual void Eval(Evaluator* ev, string* s) const override {
    for (Value* v : vals_) {
      v->Eval(ev, s);
    }
  }

  virtual string DebugString_() const override {
    string r;
    for (Value* v : vals_) {
      if (r.empty()) {
        r += "Expr(";
      } else {
        r += ", ";
      }
      r += v->DebugString();
    }
    r += ")";
    return r;
  }

  virtual Value* Compact() {
    if (vals_.size() != 1) {
      return this;
    }
    Value* r = vals_[0];
    vals_.clear();
    delete this;
    return r;
  }

 private:
  vector<Value*> vals_;
};

class VarRef : public Value {
 public:
  explicit VarRef(Value* n)
      : name_(n) {
  }
  virtual ~VarRef() {
    delete name_;
  }

  virtual shared_ptr<string> Eval(Evaluator* ev) const override {
    shared_ptr<string> name = name_->Eval(ev);
    Var* v = ev->LookupVar(*name);
    return v->Eval(ev);
  }

  virtual void Eval(Evaluator* ev, string* s) const override {
    shared_ptr<string> name = name_->Eval(ev);
    Var* v = ev->LookupVar(*name);
    v->Eval(ev, s);
  }

  virtual string DebugString_() const override {
    return StringPrintf("VarRef(%s)", name_->DebugString().c_str());
  }

 private:
  Value* name_;
};

class VarSubst : public Value {
 public:
  explicit VarSubst(Value* n, Value* p, Value* s)
      : name_(n), pat_(p), subst_(s) {
  }
  virtual ~VarSubst() {
    delete name_;
    delete pat_;
    delete subst_;
  }

  virtual void Eval(Evaluator* ev, string* s) const override {
    shared_ptr<string> name = name_->Eval(ev);
    Var* v = ev->LookupVar(*name);
    shared_ptr<string> value = v->Eval(ev);
    shared_ptr<string> pat_str = pat_->Eval(ev);
    shared_ptr<string> subst = subst_->Eval(ev);
    WordWriter ww(s);
    Pattern pat(*pat_str);
    for (StringPiece tok : WordScanner(*value)) {
      ww.MaybeAddWhitespace();
      pat.AppendSubstRef(tok, *subst, s);
    }
  }

  virtual string DebugString_() const override {
    return StringPrintf("VarSubst(%s:%s=%s)",
                        name_->DebugString().c_str(),
                        pat_->DebugString().c_str(),
                        subst_->DebugString().c_str());
  }

 private:
  Value* name_;
  Value* pat_;
  Value* subst_;
};

class Func : public Value {
 public:
  explicit Func(FuncInfo* fi)
      : fi_(fi) {
  }

  ~Func() {
    for (Value* a : args_)
      delete a;
  }

  virtual void Eval(Evaluator* ev, string* s) const override {
    LOG("Invoke func %s(%s)", name(), JoinValues(args_, ",").c_str());
    fi_->func(args_, ev, s);
  }

  virtual string DebugString_() const override {
    return StringPrintf("Func(%s %s)",
                        fi_->name,
                        JoinValues(args_, ",").c_str());
  }

  void AddArg(Value* v) {
    args_.push_back(v);
  }

  const char* name() const { return fi_->name; }
  int arity() const { return fi_->arity; }
  int min_arity() const { return fi_->min_arity; }
  bool trim_space() const { return fi_->trim_space; }
  bool trim_right_space_1st() const { return fi_->trim_right_space_1st; }

 private:
  FuncInfo* fi_;
  vector<Value*> args_;
};

static char CloseParen(char c) {
  switch (c) {
    case '(':
      return ')';
    case '{':
      return '}';
  }
  return 0;
}

static size_t SkipSpaces(StringPiece s, const char* terms) {
  for (size_t i = 0; i < s.size(); i++) {
    char c = s[i];
    if ((c != ' ' && c != '\t') || strchr(terms, c))
      return i;
  }
  return s.size();
}

Value* ParseFunc(Func* f, StringPiece s, size_t i, char* terms,
                 size_t* index_out) {
  terms[1] = ',';
  terms[2] = '\0';
  i += SkipSpaces(s.substr(i), terms);
  if (i == s.size()) {
    *index_out = i;
    return f;
  }

  int nargs = 1;
  while (true) {
    if (f->arity() && nargs >= f->arity()) {
      terms[1] = '\0';  // Drop ','.
    }

    if (f->trim_space()) {
      while (i < s.size() && isspace(s[i]))
        i++;
    }
    const bool trim_right_space = (f->trim_space() ||
                                   (nargs == 1 && f->trim_right_space_1st()));
    size_t n;
    Value* v = ParseExprImpl(s.substr(i), terms, ParseExprOpt::NORMAL,
                             &n, trim_right_space);
    // TODO: concatLine???
    f->AddArg(v);
    i += n;
    nargs++;
    if (s[i] == terms[0]) {
      i++;
      break;
    }
    i++;  // Should be ','.
    if (i == s.size())
      break;
  }

  if (nargs <= f->min_arity()) {
    // TODO: Show filename and line number.
    ERROR("*** insufficient number of arguments (%d) to function `%s'.",
          nargs - 1, f->name());
  }

  *index_out = i;
  return f;
}

Value* ParseDollar(StringPiece s, size_t* index_out) {
  CHECK(s.size() >= 2);
  CHECK(s[0] == '$');
  CHECK(s[1] != '$');

  char cp = CloseParen(s[1]);
  if (cp == 0) {
    *index_out = 2;
    return new VarRef(new Literal(s.substr(1, 1)));
  }

  char terms[] = {cp, ':', ' ', 0};
  for (size_t i = 2;;) {
    size_t n;
    Value* vname = ParseExprImpl(s.substr(i), terms, ParseExprOpt::NORMAL, &n);
    i += n;
    if (s[i] == cp) {
      *index_out = i + 1;
      return new VarRef(vname);
    }

    if (s[i] == ' ') {
      // ${func ...}
      if (Literal* lit = reinterpret_cast<Literal*>(vname)) {
        if (FuncInfo* fi = GetFuncInfo(lit->val())) {
          delete lit;
          Func* func = new Func(fi);
          return ParseFunc(func, s, i+1, terms, index_out);
        }
      }

      // Not a function. Drop ' ' from |terms| and parse it
      // again. This is inefficient, but this code path should be
      // rarely used.
      delete vname;
      terms[2] = 0;
      i = 2;
      continue;
    }

    if (s[i] == ':') {
      terms[2] = '\0';
      terms[1] = '=';
      size_t n;
      Value* pat = ParseExprImpl(s.substr(i+1), terms, ParseExprOpt::NORMAL,
                                 &n);
      i += 1 + n;
      if (s[i] == cp) {
        Expr* v = new Expr;
        v->AddValue(vname);
        v->AddValue(new Literal(":"));
        v->AddValue(pat);
        *index_out = i + 1;
        return new VarRef(v);
      }

      terms[1] = '\0';
      Value* subst = ParseExprImpl(s.substr(i+1), terms, ParseExprOpt::NORMAL,
                                   &n);
      i += 1 + n;
      *index_out = i + 1;
      return new VarSubst(vname->Compact(), pat, subst);
    }

    CHECK(false);
  }
}

Value* ParseExprImpl(StringPiece s, const char* terms, ParseExprOpt opt,
                     size_t* index_out, bool trim_right_space) {
  // TODO: A faulty optimization.
#if 0
  char specials[] = "$(){}\\\n";
  size_t found = s.find_first_of(specials);
  if (found == string::npos) {
    *index_out = s.size();
    return new Literal(s);
  }
  if (terms && strchr(terms, s[found])) {
    *index_out = found;
    return new Literal(s.substr(0, found));
  }
#endif
  if (s.get(s.size() - 1) == '\r')
    s.remove_suffix(1);

  Expr* r = new Expr;
  size_t b = 0;
  char save_paren = 0;
  int paren_depth = 0;
  size_t i;
  for (i = 0; i < s.size(); i++) {
    char c = s[i];
    if (terms && strchr(terms, c)) {
      break;
    }

    // Handle a comment.
    if (!terms && c == '#' && opt == ParseExprOpt::NORMAL) {
      if (i > b)
        r->AddValue(new Literal(s.substr(b, i-b)));
      bool was_backslash = false;
      for (; i < s.size() && !(s[i] == '\n' && !was_backslash); i++) {
        was_backslash = !was_backslash && s[i] == '\\';
      }
      *index_out = i;
      return r->Compact();
    }

    if (c == '$') {
      if (i + 1 >= s.size()) {
        break;
      }

      if (i > b)
        r->AddValue(new Literal(s.substr(b, i-b)));

      if (s[i+1] == '$') {
        r->AddValue(new Literal(STRING_PIECE("$")));
        i += 2;
        b = i;
        continue;
      }

      if (terms && strchr(terms, s[i+1])) {
        *index_out = i + 1;
        return r->Compact();
      }

      size_t n;
      Value* v = ParseDollar(s.substr(i), &n);
      i += n;
      b = i;
      i--;
      r->AddValue(v);
      continue;
    }

    if (c == '(' || c == '{') {
      char cp = CloseParen(c);
      if (terms && terms[0] == cp) {
        paren_depth++;
        save_paren = cp;
        terms++;
      } else if (cp == save_paren) {
        paren_depth++;
      }
      continue;
    }

    if (c == save_paren) {
      paren_depth--;
      if (paren_depth == 0) {
        terms--;
        save_paren = 0;
      }
    }

    if (c == '\\' && i + 1 < s.size() && opt != ParseExprOpt::COMMAND) {
      char n = s[i+1];
      if (n == '\\') {
        i++;
        continue;
      }
      if (n == '#' && opt == ParseExprOpt::NORMAL) {
        r->AddValue(new Literal(s.substr(b, i-b)));
        i++;
        b = i;
        continue;
      }
      if (n == '\r' || n == '\n') {
        if (i > b) {
          r->AddValue(new Literal(TrimRightSpace(s.substr(b, i-b))));
        }
        r->AddValue(new Literal(STRING_PIECE(" ")));
        for (i++; i < s.size(); i++) {
          if (!isspace(s[i]) &&
              (s[i] != '\\' || (s.get(i+1) != '\r' && s.get(i+1) != '\n'))) {
            break;
          }
        }
        b = i;
        i--;
      }
    }
  }

  if (i > b) {
    StringPiece rest = s.substr(b, i-b);
    if (trim_right_space)
      rest = TrimRightSpace(rest);
    if (!rest.empty())
      r->AddValue(new Literal(rest));
  }
  *index_out = i;
  return r->Compact();
}

Value* ParseExpr(StringPiece s, ParseExprOpt opt) {
  size_t n;
  return ParseExprImpl(s, NULL, opt, &n);
}

Value* ParseExprUntilComma(StringPiece s, size_t* index_out) {
  char terms[] = {',', '\0'};
  return ParseExprImpl(s, terms, ParseExprOpt::NORMAL, index_out);
}

string JoinValues(const vector<Value*>& vals, const char* sep) {
  vector<string> val_strs;
  for (Value* v : vals) {
    val_strs.push_back(v->DebugString());
  }
  return JoinStrings(val_strs, sep);
}

Value* NewExpr2(Value* v1, Value* v2) {
  Expr* e = new Expr();
  e->AddValue(v1);
  e->AddValue(v2);
  return e;
}

Value* NewExpr3(Value* v1, Value* v2, Value* v3) {
  Expr* e = new Expr();
  e->AddValue(v1);
  e->AddValue(v2);
  e->AddValue(v3);
  return e;
}

Value* NewLiteral(StringPiece s) {
  return new Literal(s);
}
