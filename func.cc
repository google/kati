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

#include "func.h"

#include <glob.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>

#include <algorithm>
#include <iterator>
#include <memory>
#include <unordered_map>

#include "ast.h"
#include "eval.h"
#include "log.h"
#include "parser.h"
#include "strutil.h"
#include "var.h"

namespace {

void PatsubstFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pat_str = args[0]->Eval(ev);
  shared_ptr<string> repl = args[1]->Eval(ev);
  shared_ptr<string> str = args[2]->Eval(ev);
  WordWriter ww(s);
  Pattern pat(*pat_str);
  for (StringPiece tok : WordScanner(*str)) {
    ww.MaybeAddWhitespace();
    pat.AppendSubst(tok, *repl, s);
  }
}

void StripFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> str = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*str)) {
    ww.Write(tok);
  }
}

void SubstFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pat = args[0]->Eval(ev);
  shared_ptr<string> repl = args[1]->Eval(ev);
  shared_ptr<string> str = args[2]->Eval(ev);
  size_t index = 0;
  while (index < str->size()) {
    size_t found = str->find(*pat, index);
    if (found == string::npos)
      break;
    AppendString(StringPiece(*str).substr(index, found - index), s);
    AppendString(*repl, s);
    index = found + pat->size();
  }
  AppendString(StringPiece(*str).substr(index), s);
}

void FindstringFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> find = args[0]->Eval(ev);
  shared_ptr<string> in = args[1]->Eval(ev);
  if (in->find(*find) != string::npos)
    AppendString(*find, s);
}

void FilterFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pat_buf = args[0]->Eval(ev);
  shared_ptr<string> text = args[1]->Eval(ev);
  vector<Pattern> pats;
  for (StringPiece pat : WordScanner(*pat_buf)) {
    pats.push_back(Pattern(pat));
  }
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    for (const Pattern& pat : pats) {
      if (pat.Match(tok)) {
        ww.Write(tok);
        break;
      }
    }
  }
}

void FilterOutFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pat_buf = args[0]->Eval(ev);
  shared_ptr<string> text = args[1]->Eval(ev);
  vector<Pattern> pats;
  for (StringPiece pat : WordScanner(*pat_buf)) {
    pats.push_back(Pattern(pat));
  }
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    bool matched = false;
    for (const Pattern& pat : pats) {
      if (pat.Match(tok)) {
        matched = true;
        break;
      }
    }
    if (!matched)
      ww.Write(tok);
  }
}

void SortFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> list = args[0]->Eval(ev);
  vector<StringPiece> toks;
  WordScanner(*list).Split(&toks);
  sort(toks.begin(), toks.end());
  WordWriter ww(s);
  StringPiece prev;
  for (StringPiece tok : toks) {
    if (prev != tok) {
      ww.Write(tok);
      prev = tok;
    }
  }
}

static int GetNumericValueForFunc(const string& buf) {
  StringPiece s = TrimLeftSpace(buf);
  char* end;
  long n = strtol(s.data(), &end, 10);
  if (n < 0 || n == LONG_MAX || s.data() + s.size() != end) {
    return -1;
  }
  return n;
}

void WordFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> n_str = args[0]->Eval(ev);
  int n = GetNumericValueForFunc(*n_str);
  if (n < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric first argument to `word' function: '%s'.",
        n_str->c_str()));
  }
  if (n == 0) {
    ev->Error("*** first argument to `word' function must be greater than 0.");
  }

  shared_ptr<string> text = args[1]->Eval(ev);
  for (StringPiece tok : WordScanner(*text)) {
    n--;
    if (n == 0) {
      AppendString(tok, s);
      break;
    }
  }
}

void WordlistFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> s_str = args[0]->Eval(ev);
  int si = GetNumericValueForFunc(*s_str);
  if (si < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric first argument to `wordlist' function: '%s'.",
        s_str->c_str()));
  }
  if (si == 0) {
    ev->Error(StringPrintf(
        "*** invalid first argument to `wordlist' function: %s`",
        s_str->c_str()));
  }

  shared_ptr<string> e_str = args[1]->Eval(ev);
  int ei = GetNumericValueForFunc(*e_str);
  if (ei < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric second argument to `wordlist' function: '%s'.",
        e_str->c_str()));
  }

  shared_ptr<string> text = args[2]->Eval(ev);
  int i = 0;
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    i++;
    if (si <= i && i <= ei) {
      ww.Write(tok);
    }
  }
}

void WordsFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  WordScanner ws(*text);
  int n = 0;
  for (auto iter = ws.begin(); iter != ws.end(); ++iter)
    n++;
  char buf[32];
  sprintf(buf, "%d", n);
  *s += buf;
}

void FirstwordFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  for (StringPiece tok : WordScanner(*text)) {
    AppendString(tok, s);
    return;
  }
}

void LastwordFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  StringPiece last;
  for (StringPiece tok : WordScanner(*text)) {
    last = tok;
  }
  AppendString(last, s);
}

void JoinFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> list1 = args[0]->Eval(ev);
  shared_ptr<string> list2 = args[1]->Eval(ev);
  WordScanner ws1(*list1);
  WordScanner ws2(*list2);
  WordWriter ww(s);
  for (WordScanner::Iterator iter1 = ws1.begin(), iter2 = ws2.begin();
       iter1 != ws1.end() && iter2 != ws2.end();
       ++iter1, ++iter2) {
    ww.Write(*iter1);
    // Use |AppendString| not to append extra ' '.
    AppendString(*iter2, s);
  }
}

void WildcardFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pat = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "$(/bin/ls -d ";
    *s += *pat;
    *s += " 2> /dev/null)";
    return;
  }

  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*pat)) {
    ScopedTerminator st(tok);
    // TODO: Make this faster by not always using glob.
    glob_t gl;
    glob(tok.data(), GLOB_NOSORT, NULL, &gl);
    for (size_t i = 0; i < gl.gl_pathc; i++) {
      ww.Write(gl.gl_pathv[i]);
    }
    globfree(&gl);
  }
}

void DirFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    ww.Write(Dirname(tok));
    if (tok != "/")
      s->push_back('/');
  }
}

void NotdirFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    if (tok == "/") {
      ww.Write(STRING_PIECE(""));
    } else {
      ww.Write(Basename(tok));
    }
  }
}

void SuffixFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    StringPiece suf = GetExt(tok);
    if (!suf.empty())
      ww.Write(suf);
  }
}

void BasenameFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    ww.Write(StripExt(tok));
  }
}

void AddsuffixFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> suf = args[0]->Eval(ev);
  shared_ptr<string> text = args[1]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    ww.Write(tok);
    *s += *suf;
  }
}

void AddprefixFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pre = args[0]->Eval(ev);
  shared_ptr<string> text = args[1]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    ww.Write(*pre);
    AppendString(tok, s);
  }
}

void RealpathFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "KATI_TODO(realpath)";
    return;
  }

  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    ScopedTerminator st(tok);
    char buf[PATH_MAX];
    if (realpath(tok.data(), buf))
      *s += buf;
  }
}

void AbspathFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> text = args[0]->Eval(ev);
  WordWriter ww(s);
  string buf;
  for (StringPiece tok : WordScanner(*text)) {
    AbsPath(tok, &buf);
    ww.Write(buf);
  }
}

void IfFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> cond = args[0]->Eval(ev);
  if (cond->empty()) {
    if (args.size() > 2)
      args[2]->Eval(ev, s);
  } else {
    args[1]->Eval(ev, s);
  }
}

void AndFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> cond;
  for (Value* a : args) {
    cond = a->Eval(ev);
    if (cond->empty())
      return;
  }
  if (cond.get()) {
    *s += *cond;
  }
}

void OrFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  for (Value* a : args) {
    shared_ptr<string> cond = a->Eval(ev);
    if (!cond->empty()) {
      *s += *cond;
      return;
    }
  }
}

void ValueFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(*var_name);
  AppendString(var->String().as_string(), s);
}

void EvalFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  // TODO: eval leaks everything... for now.
  //shared_ptr<string> text = args[0]->Eval(ev);
  string* text = new string;
  args[0]->Eval(ev, text);
  vector<AST*> asts;
  Parse(*text, ev->loc(), &asts);
  for (AST* ast : asts) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(ev);
    //delete ast;
  }
}

void ShellFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> cmd = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "$(";
    *s += *cmd;
    *s += ")";
    return;
  }

  LOG("ShellFunc: %s", cmd->c_str());
  string out;
  // TODO: Handle $(SHELL).
  FILE* fp = popen(cmd->c_str(), "r");
  while (true) {
    char buf[4096];
    size_t r = fread(buf, 1, 4096, fp);
    out.append(buf, buf+r);
    if (r == 0) {
      fclose(fp);
      break;
    }
  }

  while (out[out.size()-1] == '\n')
    out.pop_back();
  for (size_t i = 0; i < out.size(); i++) {
    if (out[i] == '\n')
      out[i] = ' ';
  }
  *s += out;
}

void CallFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  static const char* tmpvar_names[] = {
    "0", "1",  "2",  "3",  "4",  "5",  "6",  "7",  "8",  "9"
  };

  shared_ptr<string> func_name = args[0]->Eval(ev);
  Var* func = ev->LookupVar(*func_name);
  vector<unique_ptr<SimpleVar>> av;
  for (size_t i = 1; i < args.size(); i++) {
    unique_ptr<SimpleVar> s(
        new SimpleVar(args[i]->Eval(ev), VarOrigin::AUTOMATIC));
    av.push_back(move(s));
  }
  vector<unique_ptr<ScopedVar>> sv;
  for (size_t i = 1; i < args.size(); i++) {
    sv.push_back(move(unique_ptr<ScopedVar>(
        new ScopedVar(ev->mutable_vars(), tmpvar_names[i], av[i-1].get()))));
  }
  func->Eval(ev, s);
}

void ForeachFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> varname = args[0]->Eval(ev);
  shared_ptr<string> list = args[1]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*list)) {
    unique_ptr<SimpleVar> v(new SimpleVar(
        make_shared<string>(tok.data(), tok.size()), VarOrigin::AUTOMATIC));
    ScopedVar sv(ev->mutable_vars(), *varname, v.get());
    ww.MaybeAddWhitespace();
    args[2]->Eval(ev, s);
  }
}

void OriginFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(*var_name);
  *s += GetOriginStr(var->Origin());
}

void FlavorFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(*var_name);
  *s += var->Flavor();
}

void InfoFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "KATI_TODO(info)";
    return;
  }
  printf("%s\n", a->c_str());
  fflush(stdout);
}

void WarningFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "KATI_TODO(warning)";
    return;
  }
  printf("%s:%d: %s\n", LOCF(ev->loc()), a->c_str());
  fflush(stdout);
}

void ErrorFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "KATI_TODO(error)";
    return;
  }
  ev->Error(StringPrintf("*** %s.", a->c_str()));
}

FuncInfo g_func_infos[] = {
  { "patsubst", &PatsubstFunc, 3, 3, false, false },
  { "strip", &StripFunc, 1, 1, false, false },
  { "subst", &SubstFunc, 3, 3, false, false },
  { "findstring", &FindstringFunc, 2, 2, false, false },
  { "filter", &FilterFunc, 2, 2, false, false },
  { "filter-out", &FilterOutFunc, 2, 2, false, false },
  { "sort", &SortFunc, 1, 1, false, false },
  { "word", &WordFunc, 2, 2, false, false },
  { "wordlist", &WordlistFunc, 3, 3, false, false },
  { "words", &WordsFunc, 1, 1, false, false },
  { "firstword", &FirstwordFunc, 1, 1, false, false },
  { "lastword", &LastwordFunc, 1, 1, false, false },

  { "join", &JoinFunc, 2, 2, false, false },
  { "wildcard", &WildcardFunc, 1, 1, false, false },
  { "dir", &DirFunc, 1, 1, false, false },
  { "notdir", &NotdirFunc, 1, 1, false, false },
  { "suffix", &SuffixFunc, 1, 1, false, false },
  { "basename", &BasenameFunc, 1, 1, false, false },
  { "addsuffix", &AddsuffixFunc, 2, 2, false, false },
  { "addprefix", &AddprefixFunc, 2, 2, false, false },
  { "realpath", &RealpathFunc, 1, 1, false, false },
  { "abspath", &AbspathFunc, 1, 1, false, false },

  { "if", &IfFunc, 3, 2, false, true },
  { "and", &AndFunc, 0, 0, true, false },
  { "or", &OrFunc, 0, 0, true, false },

  { "value", &ValueFunc, 1, 1, false, false },
  { "eval", &EvalFunc, 1, 1, false, false },
  { "shell", &ShellFunc, 1, 1, false, false },
  { "call", &CallFunc, 0, 0, false, false },
  { "foreach", &ForeachFunc, 3, 3, false, false },

  { "origin", &OriginFunc, 1, 1, false, false },
  { "flavor", &FlavorFunc, 1, 1, false, false },

  { "info", &InfoFunc, 1, 1, false, false },
  { "warning", &WarningFunc, 1, 1, false, false },
  { "error", &ErrorFunc, 1, 1, false, false },
};

unordered_map<StringPiece, FuncInfo*>* g_func_info_map;

}  // namespace

void InitFuncTable() {
  g_func_info_map = new unordered_map<StringPiece, FuncInfo*>;
  for (size_t i = 0; i < sizeof(g_func_infos) / sizeof(g_func_infos[0]); i++) {
    FuncInfo* fi = &g_func_infos[i];
    bool ok = g_func_info_map->insert(make_pair(Intern(fi->name), fi)).second;
    CHECK(ok);
  }
}

void QuitFuncTable() {
  delete g_func_info_map;
}

FuncInfo* GetFuncInfo(StringPiece name) {
  auto found = g_func_info_map->find(name);
  if (found == g_func_info_map->end())
    return NULL;
  return found->second;
}
