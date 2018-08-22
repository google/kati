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

#include "expr.h"

#include <vector>

#include "eval.h"
#include "func.h"
#include "log.h"
#include "stringprintf.h"
#include "strutil.h"
#include "var.h"

Evaluable::Evaluable() {}

Evaluable::~Evaluable() {}

string Evaluable::Eval(Evaluator* ev) const {
  string s;
  Eval(ev, &s);
  return s;
}

Value::Value() {}

Value::~Value() {}

string Value::DebugString(const Value *v) {
  return v ? NoLineBreak(v->DebugString_()) : "(null)";
}

class Literal : public Value {
 public:
  explicit Literal(StringPiece s) : s_(s) {}

  StringPiece val() const { return s_; }

  virtual void Eval(Evaluator* ev, string* s) const override {
    ev->CheckStack();
    s->append(s_.begin(), s_.end());
  }

  virtual bool IsLiteral() const override { return true; }
  virtual StringPiece GetLiteralValueUnsafe() const override { return s_; }

  virtual string DebugString_() const override { return s_.as_string(); }

 private:
  StringPiece s_;
};

class ValueList : public Value {
 public:
  ValueList() {}

  ValueList(Value *v1, Value *v2, Value *v3)
      :ValueList(){
    vals_.reserve(3);
    vals_.push_back(v1);
    vals_.push_back(v2);
    vals_.push_back(v3);
  }

  ValueList(Value *v1, Value *v2):
      ValueList() {
    vals_.reserve(2);
    vals_.push_back(v1);
    vals_.push_back(v2);
  }

  ValueList(vector<Value *> *values):ValueList() {
    values->shrink_to_fit();
    values->swap(vals_);
  }


  virtual ~ValueList() {
    for (Value* v : vals_) {
      delete v;
    }
  }

  virtual void Eval(Evaluator* ev, string* s) const override {
    ev->CheckStack();
    for (Value* v : vals_) {
      v->Eval(ev, s);
    }
  }

  virtual string DebugString_() const override {
    string r;
    for (Value* v : vals_) {
      if (r.empty()) {
        r += "ValueList(";
      } else {
        r += ", ";
      }
      r += DebugString(v);
    }
    if (!r.empty())
      r += ")";
    return r;
  }

 private:
  vector<Value*> vals_;
};

class SymRef : public Value {
 public:
  explicit SymRef(Symbol n) : name_(n) {}
  virtual ~SymRef() {}

  virtual void Eval(Evaluator* ev, string* s) const override {
    ev->CheckStack();
    Var* v = ev->LookupVar(name_);
    v->Used(ev, name_);
    v->Eval(ev, s);
  }

  virtual string DebugString_() const override {
    return StringPrintf("SymRef(%s)", name_.c_str());
  }

 private:
  Symbol name_;
};

class VarRef : public Value {
 public:
  explicit VarRef(Value* n) : name_(n) {}
  virtual ~VarRef() { delete name_; }

  virtual void Eval(Evaluator* ev, string* s) const override {
    ev->CheckStack();
    ev->IncrementEvalDepth();
    const string&& name = name_->Eval(ev);
    ev->DecrementEvalDepth();
    Symbol sym = Intern(name);
    Var* v = ev->LookupVar(sym);
    v->Used(ev, sym);
    v->Eval(ev, s);
  }

  virtual string DebugString_() const override {
    return StringPrintf("VarRef(%s)", Value::DebugString(name_).c_str());
  }

 private:
  Value* name_;
};

class VarSubst : public Value {
 public:
  explicit VarSubst(Value* n, Value* p, Value* s)
      : name_(n), pat_(p), subst_(s) {}
  virtual ~VarSubst() {
    delete name_;
    delete pat_;
    delete subst_;
  }

  virtual void Eval(Evaluator* ev, string* s) const override {
    ev->CheckStack();
    ev->IncrementEvalDepth();
    const string&& name = name_->Eval(ev);
    Symbol sym = Intern(name);
    Var* v = ev->LookupVar(sym);
    const string&& pat_str = pat_->Eval(ev);
    const string&& subst = subst_->Eval(ev);
    ev->DecrementEvalDepth();
    v->Used(ev, sym);
    const string&& value = v->Eval(ev);
    WordWriter ww(s);
    Pattern pat(pat_str);
    for (StringPiece tok : WordScanner(value)) {
      ww.MaybeAddWhitespace();
      pat.AppendSubstRef(tok, subst, s);
    }
  }

  virtual string DebugString_() const override {
    return StringPrintf("VarSubst(%s:%s=%s)", Value::DebugString(name_).c_str(),
                        Value::DebugString(pat_).c_str(),
                        Value::DebugString(subst_).c_str());
  }

 private:
  Value* name_;
  Value* pat_;
  Value* subst_;
};

class Func : public Value {
 public:
  explicit Func(FuncInfo* fi) : fi_(fi) {}

  ~Func() {
    for (Value* a : args_)
      delete a;
  }

  virtual void Eval(Evaluator* ev, string* s) const override {
    ev->CheckStack();
    LOG("Invoke func %s(%s)", name(), JoinValues(args_, ",").c_str());
    ev->IncrementEvalDepth();
    fi_->func(args_, ev, s);
    ev->DecrementEvalDepth();
  }

  virtual string DebugString_() const override {
    return StringPrintf("Func(%s %s)", fi_->name,
                        JoinValues(args_, ",").c_str());
  }

  void AddArg(Value* v) { args_.push_back(v); }

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
    if (strchr(terms, c))
      return i;
    if (!isspace(c)) {
      if (c != '\\')
        return i;
      char n = s.get(i + 1);
      if (n != '\r' && n != '\n')
        return i;
    }
  }
  return s.size();
}

Value* Value::NewExpr(Value* v1, Value* v2) {
  return new ValueList(v1, v2);
}

Value* Value::NewExpr(Value* v1, Value* v2, Value* v3) {
  return new ValueList(v1, v2, v3);
}

Value* Value::NewExpr(vector<Value *> *values) {
  if (values->size() == 1) {
    Value *v = (*values)[0];
    values->clear();
    return v;
  }
  return new ValueList(values);
}

Value* Value::NewLiteral(StringPiece s) {
  return new Literal(s);
}

bool ShouldHandleComments(ParseExprOpt opt) {
  return opt != ParseExprOpt::DEFINE && opt != ParseExprOpt::COMMAND;
}

void ParseFunc(const Loc& loc,
               Func* f,
               StringPiece s,
               size_t i,
               char* terms,
               size_t* index_out) {
  terms[1] = ',';
  terms[2] = '\0';
  i += SkipSpaces(s.substr(i), terms);
  if (i == s.size()) {
    *index_out = i;
    return;
  }

  int nargs = 1;
  while (true) {
    if (f->arity() && nargs >= f->arity()) {
      terms[1] = '\0';  // Drop ','.
    }

    if (f->trim_space()) {
      for (; i < s.size(); i++) {
        if (isspace(s[i]))
          continue;
        if (s[i] == '\\') {
          char c = s.get(i + 1);
          if (c == '\r' || c == '\n')
            continue;
        }
        break;
      }
    }
    const bool trim_right_space =
        (f->trim_space() || (nargs == 1 && f->trim_right_space_1st()));
    size_t n;
    Value* v = ParseExprImpl(loc, s.substr(i), terms, ParseExprOpt::FUNC, &n,
                             trim_right_space);
    // TODO: concatLine???
    f->AddArg(v);
    i += n;
    if (i == s.size()) {
      ERROR_LOC(loc,
                "*** unterminated call to function '%s': "
                "missing '%c'.",
                f->name(), terms[0]);
    }
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
    ERROR_LOC(loc,
              "*** insufficient number of arguments (%d) to function `%s'.",
              nargs - 1, f->name());
  }

  *index_out = i;
  return;
}

Value* ParseDollar(const Loc& loc, StringPiece s, size_t* index_out) {
  CHECK(s.size() >= 2);
  CHECK(s[0] == '$');
  CHECK(s[1] != '$');

  char cp = CloseParen(s[1]);
  if (cp == 0) {
    *index_out = 2;
    return new SymRef(Intern(s.substr(1, 1)));
  }

  char terms[] = {cp, ':', ' ', 0};
  for (size_t i = 2;;) {
    size_t n;
    Value* vname =
        ParseExprImpl(loc, s.substr(i), terms, ParseExprOpt::NORMAL, &n);
    i += n;
    if (s[i] == cp) {
      *index_out = i + 1;
      if (vname->IsLiteral()) {
        Literal* lit = static_cast<Literal*>(vname);
        Symbol sym = Intern(lit->val());
        if (g_flags.enable_kati_warnings) {
          size_t found = sym.str().find_first_of(" ({");
          if (found != string::npos) {
            KATI_WARN_LOC(loc, "*warning*: variable lookup with '%c': %.*s",
                          sym.str()[found], SPF(s));
          }
        }
        Value* r = new SymRef(sym);
        delete lit;
        return r;
      }
      return new VarRef(vname);
    }

    if (s[i] == ' ' || s[i] == '\\') {
      // ${func ...}
      if (vname->IsLiteral()) {
        Literal* lit = static_cast<Literal*>(vname);
        if (FuncInfo* fi = GetFuncInfo(lit->val())) {
          delete lit;
          Func* func = new Func(fi);
          ParseFunc(loc, func, s, i + 1, terms, index_out);
          return func;
        } else {
          KATI_WARN_LOC(loc, "*warning*: unknown make function '%.*s': %.*s",
                        SPF(lit->val()), SPF(s));
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
      Value* pat =
          ParseExprImpl(loc, s.substr(i + 1), terms, ParseExprOpt::NORMAL, &n);
      i += 1 + n;
      if (s[i] == cp) {
        *index_out = i + 1;
        return new VarRef(Value::NewExpr(vname, new Literal(":"), pat));
      }

      terms[1] = '\0';
      Value* subst =
          ParseExprImpl(loc, s.substr(i + 1), terms, ParseExprOpt::NORMAL, &n);
      i += 1 + n;
      *index_out = i + 1;
      return new VarSubst(vname, pat, subst);
    }

    // GNU make accepts expressions like $((). See unmatched_paren*.mk
    // for detail.
    size_t found = s.find(cp);
    if (found != string::npos) {
      KATI_WARN_LOC(loc, "*warning*: unmatched parentheses: %.*s", SPF(s));
      *index_out = s.size();
      return new SymRef(Intern(s.substr(2, found - 2)));
    }
    ERROR_LOC(loc, "*** unterminated variable reference.");
  }
}

Value* ParseExprImpl(const Loc& loc,
                     StringPiece s,
                     const char* terms,
                     ParseExprOpt opt,
                     size_t* index_out,
                     bool trim_right_space) {
  if (s.get(s.size() - 1) == '\r')
    s.remove_suffix(1);

  size_t b = 0;
  char save_paren = 0;
  int paren_depth = 0;
  size_t i;
  vector<Value *> list;
  for (i = 0; i < s.size(); i++) {
    char c = s[i];
    if (terms && strchr(terms, c) && !save_paren) {
      break;
    }

    // Handle a comment.
    if (!terms && c == '#' && ShouldHandleComments(opt)) {
      if (i > b)
        list.push_back(new Literal(s.substr(b, i - b)));
      bool was_backslash = false;
      for (; i < s.size() && !(s[i] == '\n' && !was_backslash); i++) {
        was_backslash = !was_backslash && s[i] == '\\';
      }
      *index_out = i;
      return Value::NewExpr(&list);
    }

    if (c == '$') {
      if (i + 1 >= s.size()) {
        break;
      }

      if (i > b)
        list.push_back(new Literal(s.substr(b, i - b)));

      if (s[i + 1] == '$') {
        list.push_back(new Literal(StringPiece("$")));
        i += 1;
        b = i + 1;
        continue;
      }

      if (terms && strchr(terms, s[i + 1])) {
        *index_out = i + 1;
        return Value::NewExpr(&list);
      }

      size_t n;
      list.push_back(ParseDollar(loc, s.substr(i), &n));
      i += n;
      b = i;
      i--;
      continue;
    }

    if ((c == '(' || c == '{') && opt == ParseExprOpt::FUNC) {
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
      char n = s[i + 1];
      if (n == '\\') {
        i++;
        continue;
      }
      if (n == '#' && ShouldHandleComments(opt)) {
        list.push_back(new Literal(s.substr(b, i - b)));
        i++;
        b = i;
        continue;
      }
      if (n == '\r' || n == '\n') {
        if (terms && strchr(terms, ' ')) {
          break;
        }
        if (i > b) {
          list.push_back(new Literal(TrimRightSpace(s.substr(b, i - b))));
        }
        list.push_back(new Literal(StringPiece(" ")));
        // Skip the current escaped newline
        i += 2;
        if (n == '\r' && s.get(i) == '\n')
          i++;
        // Then continue skipping escaped newlines, spaces, and tabs
        for (; i < s.size(); i++) {
          if (s[i] == '\\' && (s.get(i + 1) == '\r' || s.get(i + 1) == '\n')) {
            i++;
            continue;
          }
          if (s[i] != ' ' && s[i] != '\t') {
            break;
          }
        }
        b = i;
        i--;
      }
    }
  }

  if (i > b) {
    StringPiece rest = s.substr(b, i - b);
    if (trim_right_space)
      rest = TrimRightSpace(rest);
    if (!rest.empty())
      list.push_back(new Literal(rest));
  }
  *index_out = i;
  return Value::NewExpr(&list);
}

Value* ParseExpr(const Loc& loc, StringPiece s, ParseExprOpt opt) {
  size_t n;
  return ParseExprImpl(loc, s, NULL, opt, &n);
}

string JoinValues(const vector<Value*>& vals, const char* sep) {
  vector<string> val_strs;
  for (Value* v : vals) {
    val_strs.push_back(Value::DebugString(v));
  }
  return JoinStrings(val_strs, sep);
}
