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

Evaluable::Evaluable(const Loc& loc) : loc_(loc) {}

Evaluable::~Evaluable() {}

std::string Evaluable::Eval(Evaluator* ev) const {
  std::string s;
  Eval(ev, &s);
  return s;
}

Value::Value(const Loc& loc) : Evaluable(loc) {}

Value::~Value() {}

std::string Value::DebugString(const Value* v) {
  return v ? NoLineBreak(v->DebugString_()) : "(null)";
}

class Literal : public Value {
 public:
  explicit Literal(std::string_view s) : Value(Loc()), s_(s) {}

  std::string_view val() const { return s_; }

  virtual bool IsFunc(Evaluator*) const override { return false; }

  virtual void Eval(Evaluator* ev, std::string* s) const override {
    ev->CheckStack();
    s->append(s_.begin(), s_.end());
  }

  virtual bool IsLiteral() const override { return true; }
  virtual std::string_view GetLiteralValueUnsafe() const override { return s_; }

  virtual std::string DebugString_() const override { return std::string(s_); }

 private:
  std::string_view s_;
};

class ValueList : public Value {
 public:
  ValueList(const Loc& loc) : Value(loc) {}

  ValueList(const Loc& loc, Value* v1, Value* v2, Value* v3) : ValueList(loc) {
    vals_.reserve(3);
    vals_.push_back(v1);
    vals_.push_back(v2);
    vals_.push_back(v3);
  }

  ValueList(const Loc& loc, Value* v1, Value* v2) : ValueList(loc) {
    vals_.reserve(2);
    vals_.push_back(v1);
    vals_.push_back(v2);
  }

  ValueList(const Loc& loc, std::vector<Value*>* values) : ValueList(loc) {
    values->shrink_to_fit();
    values->swap(vals_);
  }

  virtual ~ValueList() {
    for (Value* v : vals_) {
      delete v;
    }
  }

  virtual bool IsFunc(Evaluator* ev) const override {
    for (Value* v : vals_) {
      if (v->IsFunc(ev)) {
        return true;
      }
    }
    return false;
  }

  virtual void Eval(Evaluator* ev, std::string* s) const override {
    ev->CheckStack();
    for (Value* v : vals_) {
      v->Eval(ev, s);
    }
  }

  virtual std::string DebugString_() const override {
    std::string r;
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
  std::vector<Value*> vals_;
};

class SymRef : public Value {
 public:
  explicit SymRef(const Loc& loc, Symbol n) : Value(loc), name_(n) {}
  virtual ~SymRef() {}

  virtual bool IsFunc(Evaluator*) const override {
    // This is a heuristic, where say that if a variable has positional
    // parameters, we think it is likely to be a function. Callers can use
    // .KATI_SYMBOLS to extract variables and their values, without evaluating
    // macros that are likely to have side effects.
    return IsInteger(name_.str());
  }

  virtual void Eval(Evaluator* ev, std::string* s) const override {
    ev->CheckStack();
    Var* v = ev->LookupVarForEval(name_);
    v->Used(ev, name_);
    v->Eval(ev, s);
    ev->VarEvalComplete(name_);
  }

  virtual std::string DebugString_() const override {
    return StringPrintf("SymRef(%s)", name_.c_str());
  }

 private:
  Symbol name_;
};

class VarRef : public Value {
 public:
  explicit VarRef(const Loc& loc, Value* n) : Value(loc), name_(n) {}
  virtual ~VarRef() { delete name_; }

  virtual bool IsFunc(Evaluator*) const override {
    // This is the unhandled edge case as described in expr.h.
    return true;
  }

  virtual void Eval(Evaluator* ev, std::string* s) const override {
    ev->CheckStack();
    ev->IncrementEvalDepth();
    const std::string&& name = name_->Eval(ev);
    ev->DecrementEvalDepth();
    Symbol sym = Intern(name);
    Var* v = ev->LookupVarForEval(sym);
    v->Used(ev, sym);
    v->Eval(ev, s);
    ev->VarEvalComplete(sym);
  }

  virtual std::string DebugString_() const override {
    return StringPrintf("VarRef(%s)", Value::DebugString(name_).c_str());
  }

 private:
  Value* name_;
};

class VarSubst : public Value {
 public:
  VarSubst(const Loc& loc, Value* n, Value* p, Value* s)
      : Value(loc), name_(n), pat_(p), subst_(s) {}
  virtual ~VarSubst() {
    delete name_;
    delete pat_;
    delete subst_;
  }

  virtual bool IsFunc(Evaluator* ev) const override {
    return name_->IsFunc(ev) || pat_->IsFunc(ev) || subst_->IsFunc(ev);
  }

  virtual void Eval(Evaluator* ev, std::string* s) const override {
    ev->CheckStack();
    ev->IncrementEvalDepth();
    const std::string&& name = name_->Eval(ev);
    Symbol sym = Intern(name);
    Var* v = ev->LookupVar(sym);
    const std::string&& pat_str = pat_->Eval(ev);
    const std::string&& subst = subst_->Eval(ev);
    ev->DecrementEvalDepth();
    v->Used(ev, sym);
    const std::string&& value = v->Eval(ev);
    WordWriter ww(s);
    Pattern pat(pat_str);
    for (std::string_view tok : WordScanner(value)) {
      ww.MaybeAddWhitespace();
      pat.AppendSubstRef(tok, subst, s);
    }
  }

  virtual std::string DebugString_() const override {
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
  Func(const Loc& loc, const FuncInfo* fi) : Value(loc), fi_(fi) {}

  ~Func() {
    for (Value* a : args_)
      delete a;
  }

  virtual bool IsFunc(Evaluator*) const override { return true; }

  virtual void Eval(Evaluator* ev, std::string* s) const override {
    ScopedFrame frame(ev->Enter(FrameType::FUNCALL, fi_->name, Location()));
    ev->CheckStack();
    LOG("Invoke func %s(%s)", name(), JoinValues(args_, ",").c_str());
    ev->IncrementEvalDepth();
    fi_->func(args_, ev, s);
    ev->DecrementEvalDepth();
  }

  virtual std::string DebugString_() const override {
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
  const FuncInfo* fi_;
  std::vector<Value*> args_;
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

static size_t SkipSpaces(Loc* loc, std::string_view s, const char* terms) {
  for (size_t i = 0; i < s.size(); i++) {
    char c = s[i];
    if (strchr(terms, c)) {
      return i;
    }

    if (!isspace(c)) {
      if (c != '\\') {
        return i;
      }

      char n = i + 1 < s.size() ? s[i + 1] : 0;
      if (n != '\r' && n != '\n') {
        return i;
      }

      loc->lineno++;  // This is a backspace continuation
    }
  }
  return s.size();
}

Value* Value::NewExpr(const Loc& loc, Value* v1, Value* v2) {
  return new ValueList(loc, v1, v2);
}

Value* Value::NewExpr(const Loc& loc, Value* v1, Value* v2, Value* v3) {
  return new ValueList(loc, v1, v2, v3);
}

Value* Value::NewExpr(const Loc& loc, std::vector<Value*>* values) {
  if (values->size() == 1) {
    Value* v = (*values)[0];
    values->clear();
    return v;
  }
  return new ValueList(loc, values);
}

Value* Value::NewLiteral(std::string_view s) {
  return new Literal(s);
}

bool ShouldHandleComments(ParseExprOpt opt) {
  return opt != ParseExprOpt::DEFINE && opt != ParseExprOpt::COMMAND;
}

void ParseFunc(Loc* loc,
               Func* f,
               std::string_view s,
               size_t i,
               char* terms,
               size_t* index_out) {
  Loc start_loc = *loc;
  terms[1] = ',';
  terms[2] = '\0';
  i += SkipSpaces(loc, s.substr(i), terms);
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
        if (isspace(s[i])) {
          continue;
        }

        if (s[i] == '\\') {
          char c = i + 1 < s.size() ? s[i + 1] : 0;
          if (c == '\r' || c == '\n') {
            loc->lineno++;
            continue;
          }
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
      ERROR_LOC(start_loc,
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
    ERROR_LOC(start_loc,
              "*** insufficient number of arguments (%d) to function `%s'.",
              nargs - 1, f->name());
  }

  *index_out = i;
  return;
}

Value* ParseDollar(Loc* loc, std::string_view s, size_t* index_out) {
  CHECK(s.size() >= 2);
  CHECK(s[0] == '$');
  CHECK(s[1] != '$');

  Loc start_loc = *loc;

  char cp = CloseParen(s[1]);
  if (cp == 0) {
    *index_out = 2;
    return new SymRef(start_loc, Intern(s.substr(1, 1)));
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
          if (found != std::string::npos) {
            KATI_WARN_LOC(start_loc,
                          "*warning*: variable lookup with '%c': %.*s",
                          sym.str()[found], SPF(s));
          }
        }
        Value* r = new SymRef(start_loc, sym);
        delete lit;
        return r;
      }
      return new VarRef(start_loc, vname);
    }

    if (s[i] == ' ' || s[i] == '\\') {
      // ${func ...}
      if (vname->IsLiteral()) {
        Literal* lit = static_cast<Literal*>(vname);
        if (const FuncInfo* fi = GetFuncInfo(lit->val())) {
          delete lit;
          Func* func = new Func(start_loc, fi);
          ParseFunc(loc, func, s, i + 1, terms, index_out);
          return func;
        } else {
          KATI_WARN_LOC(start_loc,
                        "*warning*: unknown make function '%.*s': %.*s",
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
        return new VarRef(
            start_loc, Value::NewExpr(start_loc, vname, new Literal(":"), pat));
      }

      terms[1] = '\0';
      Value* subst =
          ParseExprImpl(loc, s.substr(i + 1), terms, ParseExprOpt::NORMAL, &n);
      i += 1 + n;
      *index_out = i + 1;
      return new VarSubst(start_loc, vname, pat, subst);
    }

    // GNU make accepts expressions like $((). See unmatched_paren*.mk
    // for detail.
    size_t found = s.find(cp);
    if (found != std::string::npos) {
      KATI_WARN_LOC(start_loc, "*warning*: unmatched parentheses: %.*s",
                    SPF(s));
      *index_out = s.size();
      return new SymRef(start_loc, Intern(s.substr(2, found - 2)));
    }
    ERROR_LOC(start_loc, "*** unterminated variable reference.");
  }
}

Value* ParseExprImpl(Loc* loc,
                     std::string_view s,
                     const char* terms,
                     ParseExprOpt opt,
                     size_t* index_out,
                     bool trim_right_space) {
  Loc list_loc = *loc;

  if (!s.empty() && s.back() == '\r')
    s.remove_suffix(1);

  size_t b = 0;
  char save_paren = 0;
  int paren_depth = 0;
  size_t i;
  std::vector<Value*> list;
  for (i = 0; i < s.size(); i++) {
    Loc item_loc = *loc;

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
      return Value::NewExpr(item_loc, &list);
    }

    if (c == '$') {
      if (i + 1 >= s.size()) {
        break;
      }

      if (i > b)
        list.push_back(new Literal(s.substr(b, i - b)));

      if (s[i + 1] == '$') {
        list.push_back(new Literal(std::string_view("$")));
        i += 1;
        b = i + 1;
        continue;
      }

      if (terms && strchr(terms, s[i + 1])) {
        list.push_back(new Literal(std::string_view("$")));
        *index_out = i + 1;
        return Value::NewExpr(item_loc, &list);
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
        loc->lineno++;
        if (terms && strchr(terms, ' ')) {
          break;
        }
        if (i > b) {
          list.push_back(new Literal(TrimRightSpace(s.substr(b, i - b))));
        }
        list.push_back(new Literal(std::string_view(" ")));
        // Skip the current escaped newline
        i += 2;
        if (n == '\r' && i < s.size() && s[i] == '\n') {
          i++;
        }
        // Then continue skipping escaped newlines, spaces, and tabs
        for (; i < s.size(); i++) {
          if (s[i] == '\\' && i + 1 < s.size() &&
              (s[i + 1] == '\r' || s[i + 1] == '\n')) {
            loc->lineno++;
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
    std::string_view rest = s.substr(b, i - b);
    if (trim_right_space)
      rest = TrimRightSpace(rest);
    if (!rest.empty())
      list.push_back(new Literal(rest));
  }
  *index_out = i;
  return Value::NewExpr(list_loc, &list);
}

Value* ParseExpr(Loc* loc, std::string_view s, ParseExprOpt opt) {
  size_t n;
  return ParseExprImpl(loc, s, NULL, opt, &n);
}

std::string JoinValues(const std::vector<Value*>& vals, const char* sep) {
  std::vector<std::string> val_strs;
  val_strs.reserve(vals.size());
  for (Value* v : vals) {
    val_strs.push_back(Value::DebugString(v));
  }
  return JoinStrings(val_strs, sep);
}
