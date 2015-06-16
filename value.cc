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
    return DebugString_();
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
    shared_ptr<string> pat = pat_->Eval(ev);
    shared_ptr<string> subst = subst_->Eval(ev);
    bool needs_space = false;
    for (StringPiece tok : WordScanner(*value)) {
      if (needs_space)
        s->push_back(' ');
      else
        needs_space = true;
      AppendSubstRef(tok, *pat, *subst, s);
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

static Value* ParseExprImpl(StringPiece s, const char* terms, bool is_command,
                            size_t* index_out);

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

    size_t n;
    Value* v = ParseExprImpl(s.substr(i), terms, false, &n);
    // TODO: concatLine???
    // TODO: trimLiteralSpace for conditional functions.
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
    Value* vname = ParseExprImpl(s.substr(i), terms, false, &n);
    i += n;
    if (s[i] == cp) {
      *index_out = i + 1;
      return new VarRef(vname);
    }

    if (s[i] == ' ') {
      // ${func ...}
      if (Literal* lit = reinterpret_cast<Literal*>(vname)) {
        if (FuncInfo* fi = GetFuncInfo(lit->val())) {
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
      Value* pat = ParseExprImpl(s.substr(i+1), terms, false, &n);
      i += 1 + n;
      if (s[i] == cp) {
        Expr* v = new Expr;
        v->AddValue(vname);
        v->AddValue(new Literal(":"));
        v->AddValue(pat);
        return new VarRef(v);
      }

      terms[1] = '\0';
      Value* subst = ParseExprImpl(s.substr(i+1), terms, false, &n);
      i += 1 + n;
      return new VarSubst(vname->Compact(), pat, subst);
    }

    CHECK(false);
  }
}

static Value* ParseExprImpl(StringPiece s, const char* terms, bool is_command,
                            size_t* index_out) {
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
    if (!terms && c == '#' && !is_command) {
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

    if (c == '\\' && i + 1 < s.size() && !is_command) {
      char n = s[i+1];
      if (n == '\\') {
        i++;
        continue;
      }
      if (n == '\r' || n == '\n') {
        // TODO
        CHECK(false);
      }
    }
  }

  if (i > b)
    r->AddValue(new Literal(s.substr(b, i-b)));
  *index_out = i;
  return r->Compact();
}

Value* ParseExpr(StringPiece s, bool is_command) {
  size_t n;
  return ParseExprImpl(s, NULL, is_command, &n);
}

string JoinValues(const vector<Value*> vals, const char* sep) {
  vector<string> val_strs;
  for (Value* v : vals) {
    val_strs.push_back(v->DebugString());
  }
  return JoinStrings(val_strs, sep);
}

Value* NewExpr3(Value* v1, Value* v2, Value* v3) {
  Expr* e = new Expr();
  e->AddValue(v1);
  e->AddValue(v2);
  e->AddValue(v3);
  return e;
}

Value* NewLiteral(const char* s) {
  return new Literal(s);
}
