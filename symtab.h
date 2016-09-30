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

#ifndef SYMTAB_H_
#define SYMTAB_H_

#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

extern vector<string*>* g_symbols;

class Symtab;
class Var;

class Symbol {
 public:
  struct IsUninitialized {};
  explicit Symbol(IsUninitialized)
    : v_(-1) {
  }

  const string& str() const {
    return *((*g_symbols)[v_]);
  }

  const char* c_str() const {
    return str().c_str();
  }

  bool empty() const { return !v_; }

  int val() const { return v_; }

  char get(size_t i) const {
    const string& s = str();
    if (i >= s.size())
      return 0;
    return s[i];
  }

  bool IsValid() const { return v_ >= 0; }

  Var* GetGlobalVar() const;
  void SetGlobalVar(Var* v, bool is_override = false, bool* readonly = nullptr) const;

 private:
  explicit Symbol(int v);

  int v_;

  friend class Symtab;
};

class ScopedGlobalVar {
 public:
  ScopedGlobalVar(Symbol name, Var* var);
  ~ScopedGlobalVar();

 private:
  Symbol name_;
  Var* orig_;
};

inline bool operator==(const Symbol& x, const Symbol& y) {
  return x.val() == y.val();
}

inline bool operator<(const Symbol& x, const Symbol& y) {
  return x.val() < y.val();
}

namespace std {
template<> struct hash<Symbol> {
  size_t operator()(const Symbol& s) const {
    return s.val();
  }
};
}

extern Symbol kEmptySym;
extern Symbol kShellSym;

void InitSymtab();
void QuitSymtab();
Symbol Intern(StringPiece s);

string JoinSymbols(const vector<Symbol>& syms, const char* sep);

#endif  // SYMTAB_H_
