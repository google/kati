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

//#define ENABLE_TID_CHECK

#include "symtab.h"

#ifdef ENABLE_TID_CHECK
#include <pthread.h>
#endif
#include <string.h>

#include <unordered_map>

#include "log.h"
#include "strutil.h"
#include "var.h"

struct SymbolData {
  SymbolData() : gv(Var::Undefined()) {}

  Var* gv;
};

std::vector<std::string*>* g_symbols;
static std::vector<SymbolData> g_symbol_data;

Symbol kEmptySym;
Symbol kShellSym;
Symbol kAllowRulesSym;
Symbol kKatiReadonlySym;
Symbol kVariablesSym;
Symbol kKatiSymbolsSym;

Symbol::Symbol(int v) : v_(v) {}

Var* Symbol::PeekGlobalVar() const {
  if (static_cast<size_t>(v_) >= g_symbol_data.size()) {
    return Var::Undefined();
  }
  return g_symbol_data[v_].gv;
}

Var* Symbol::GetGlobalVar() const {
  if (static_cast<size_t>(v_) >= g_symbol_data.size()) {
    g_symbol_data.resize(v_ + 1);
  }
  Var* v = g_symbol_data[v_].gv;
  if (v->Origin() == VarOrigin::ENVIRONMENT ||
      v->Origin() == VarOrigin::ENVIRONMENT_OVERRIDE) {
    Vars::add_used_env_vars(*this);
  }
  return v;
}

void Symbol::SetGlobalVar(Var* v, bool is_override, bool* readonly) const {
  if (static_cast<size_t>(v_) >= g_symbol_data.size()) {
    g_symbol_data.resize(v_ + 1);
  }
  Var* orig = g_symbol_data[v_].gv;
  if (orig->ReadOnly()) {
    if (readonly != nullptr)
      *readonly = true;
    else
      ERROR("*** cannot assign to readonly variable: %s", c_str());
    return;
  } else if (readonly != nullptr) {
    *readonly = false;
  }
  if (!is_override && (orig->Origin() == VarOrigin::OVERRIDE ||
                       orig->Origin() == VarOrigin::ENVIRONMENT_OVERRIDE)) {
    return;
  }
  if (orig->Origin() == VarOrigin::COMMAND_LINE &&
      v->Origin() == VarOrigin::FILE) {
    return;
  }
  if (orig->Origin() == VarOrigin::AUTOMATIC) {
    ERROR("overriding automatic variable is not implemented yet");
  }
  if (orig->IsDefined())
    delete orig;
  g_symbol_data[v_].gv = v;
}

ScopedGlobalVar::ScopedGlobalVar(Symbol name, Var* var)
    : name_(name), orig_(NULL) {
  orig_ = name.GetGlobalVar();
  g_symbol_data[name_.val()].gv = var;
}

ScopedGlobalVar::~ScopedGlobalVar() {
  g_symbol_data[name_.val()].gv = orig_;
}

class Symtab {
 public:
  Symtab() {
#ifdef ENABLE_TID_CHECK
    tid_ = pthread_self();
#endif

    CHECK(g_symbols == NULL);
    g_symbols = &symbols_;

    Symbol s = InternImpl("");
    CHECK(s.v_ == 0);
    CHECK(Intern("") == s);
    char b[2];
    b[1] = 0;
    for (int i = 1; i < 256; i++) {
      b[0] = i;
      s = InternImpl(b);
      CHECK(s.val() == i);
    }

    kEmptySym = Intern("");
    kShellSym = Intern("SHELL");
    Symbol shellStatusSym = Intern(".SHELLSTATUS");
    shellStatusSym.SetGlobalVar(new ShellStatusVar(), false, nullptr);
    kAllowRulesSym = Intern(".KATI_ALLOW_RULES");
    kKatiReadonlySym = Intern(".KATI_READONLY");
    kVariablesSym = Intern(".VARIABLES");
    kVariablesSym.SetGlobalVar(new VariableNamesVar(".VARIABLES", true), false,
                               nullptr);
    kKatiSymbolsSym = Intern(".KATI_SYMBOLS");
    kKatiSymbolsSym.SetGlobalVar(new VariableNamesVar(".KATI_SYMBOLS", false),
                                 false, nullptr);
  }

  ~Symtab() {
    LOG_STAT("%zu symbols", symbols_.size());
    for (std::string* s : symbols_)
      delete s;
  }

  Symbol InternImpl(std::string_view s) {
    auto found = symtab_.find(s);
    if (found != symtab_.end()) {
      return found->second;
    }
    symbols_.push_back(new std::string(s.data(), s.size()));
    Symbol sym = Symbol(symtab_.size());
    bool ok = symtab_.emplace(*symbols_.back(), sym).second;
    CHECK(ok);
    return sym;
  }

  Symbol Intern(std::string_view s) {
#ifdef ENABLE_TID_CHECK
    if (tid_ != pthread_self())
      abort();
#endif

    if (s.size() <= 1) {
      return Symbol(s.empty() ? 0 : (unsigned char)s[0]);
    }
    return InternImpl(s);
  }

  std::vector<std::string_view> GetSymbolNames(
      std::function<bool(Var*)> const& filter) {
    std::vector<std::string_view> result;
    for (auto entry : symtab_) {
      Var* var = entry.second.PeekGlobalVar();
      // The symbol table contains all interned strings, not just variables
      // which have been defined.
      if (!var->IsDefined()) {
        continue;
      }
      if (filter(var)) {
        result.push_back(entry.first);
      }
    }
    return result;
  }

 private:
  std::unordered_map<std::string_view, Symbol> symtab_;
  std::vector<std::string*> symbols_;
#ifdef ENABLE_TID_CHECK
  pthread_t tid_;
#endif
};

static Symtab g_symtab;

Symbol Intern(std::string_view s) {
  return g_symtab.Intern(s);
}

std::string JoinSymbols(const std::vector<Symbol>& syms, const char* sep) {
  std::vector<std::string> strs;
  strs.reserve(syms.size());
  for (Symbol s : syms) {
    strs.push_back(s.str());
  }
  return JoinStrings(strs, sep);
}

std::vector<std::string_view> GetSymbolNames(
    std::function<bool(Var*)> const& filter) {
  return g_symtab.GetSymbolNames(filter);
}
