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

#include "symtab.h"

#include <string.h>

#include <unordered_map>

#include "log.h"
#include "strutil.h"

vector<string>* g_symbols;

Symbol::Symbol(int v)
    : v_(v) {
}

class Symtab {
 public:
  Symtab() {
    CHECK(g_symbols == NULL);
    g_symbols = &symbols_;
    Symbol s = Intern("");
    CHECK(s.v_ == 0);
    CHECK(Intern("") == s);
  }

  Symbol Intern(StringPiece s) {
    auto found = symtab_.find(s);
    if (found != symtab_.end()) {
      return found->second;
    }
    symbols_.push_back(s.as_string());
    Symbol sym = Symbol(symtab_.size());
    bool ok = symtab_.emplace(symbols_.back(), sym).second;
    CHECK(ok);
    return sym;
  }

 private:
  unordered_map<StringPiece, Symbol> symtab_;
  vector<string> symbols_;
};

static Symtab* g_symtab;

void InitSymtab() {
  g_symtab = new Symtab;
}

void QuitSymtab() {
  delete g_symtab;
}

Symbol Intern(StringPiece s) {
  return g_symtab->Intern(s);
}

string JoinSymbols(const vector<Symbol>& syms, const char* sep) {
  vector<string> strs;
  for (Symbol s : syms) {
    strs.push_back(s.str());
  }
  return JoinStrings(strs, sep);
}
