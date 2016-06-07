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

#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

#include <algorithm>
#include <iterator>
#include <memory>
#include <unordered_map>

#include "eval.h"
#include "fileutil.h"
#include "find.h"
#include "log.h"
#include "parser.h"
#include "stats.h"
#include "stmt.h"
#include "strutil.h"
#include "symtab.h"
#include "var.h"

namespace {

// TODO: This code is very similar to
// NinjaGenerator::TranslateCommand. Factor them out.
void StripShellComment(string* cmd) {
  if (cmd->find('#') == string::npos)
    return;

  string res;
  bool prev_backslash = false;
  // Set space as an initial value so the leading comment will be
  // stripped out.
  char prev_char = ' ';
  char quote = 0;
  bool done = false;
  const char* in = cmd->c_str();
  for (; *in && !done; in++) {
    switch (*in) {
      case '#':
        if (quote == 0 && isspace(prev_char)) {
          while (in[1] && *in != '\n')
            in++;
          break;
        }

      case '\'':
      case '"':
      case '`':
        if (quote) {
          if (quote == *in)
            quote = 0;
        } else if (!prev_backslash) {
          quote = *in;
        }
        res += *in;
        break;

      case '\\':
        res += '\\';
        break;

      default:
        res += *in;
    }

    if (*in == '\\') {
      prev_backslash = !prev_backslash;
    } else {
      prev_backslash = false;
    }

    prev_char = *in;
  }
  cmd->swap(res);
}

void PatsubstFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& pat_str = args[0]->Eval(ev);
  const string&& repl = args[1]->Eval(ev);
  const string&& str = args[2]->Eval(ev);
  WordWriter ww(s);
  Pattern pat(pat_str);
  for (StringPiece tok : WordScanner(str)) {
    ww.MaybeAddWhitespace();
    pat.AppendSubst(tok, repl, s);
  }
}

void StripFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& str = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(str)) {
    ww.Write(tok);
  }
}

void SubstFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& pat = args[0]->Eval(ev);
  const string&& repl = args[1]->Eval(ev);
  const string&& str = args[2]->Eval(ev);
  if (pat.empty()) {
    *s += str;
    *s += repl;
    return;
  }
  size_t index = 0;
  while (index < str.size()) {
    size_t found = str.find(pat, index);
    if (found == string::npos)
      break;
    AppendString(StringPiece(str).substr(index, found - index), s);
    AppendString(repl, s);
    index = found + pat.size();
  }
  AppendString(StringPiece(str).substr(index), s);
}

void FindstringFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& find = args[0]->Eval(ev);
  const string&& in = args[1]->Eval(ev);
  if (in.find(find) != string::npos)
    AppendString(find, s);
}

void FilterFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& pat_buf = args[0]->Eval(ev);
  const string&& text = args[1]->Eval(ev);
  vector<Pattern> pats;
  for (StringPiece pat : WordScanner(pat_buf)) {
    pats.push_back(Pattern(pat));
  }
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    for (const Pattern& pat : pats) {
      if (pat.Match(tok)) {
        ww.Write(tok);
        break;
      }
    }
  }
}

void FilterOutFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& pat_buf = args[0]->Eval(ev);
  const string&& text = args[1]->Eval(ev);
  vector<Pattern> pats;
  for (StringPiece pat : WordScanner(pat_buf)) {
    pats.push_back(Pattern(pat));
  }
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
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
  string list;
  args[0]->Eval(ev, &list);
  COLLECT_STATS("func sort time");
  // TODO(hamaji): Probably we could use a faster string-specific sort
  // algorithm.
  vector<StringPiece> toks;
  WordScanner(list).Split(&toks);
  stable_sort(toks.begin(), toks.end());
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
  const string&& n_str = args[0]->Eval(ev);
  int n = GetNumericValueForFunc(n_str);
  if (n < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric first argument to `word' function: '%s'.",
        n_str.c_str()));
  }
  if (n == 0) {
    ev->Error("*** first argument to `word' function must be greater than 0.");
  }

  const string&& text = args[1]->Eval(ev);
  for (StringPiece tok : WordScanner(text)) {
    n--;
    if (n == 0) {
      AppendString(tok, s);
      break;
    }
  }
}

void WordlistFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& s_str = args[0]->Eval(ev);
  int si = GetNumericValueForFunc(s_str);
  if (si < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric first argument to `wordlist' function: '%s'.",
        s_str.c_str()));
  }
  if (si == 0) {
    ev->Error(StringPrintf(
        "*** invalid first argument to `wordlist' function: %s`",
        s_str.c_str()));
  }

  const string&& e_str = args[1]->Eval(ev);
  int ei = GetNumericValueForFunc(e_str);
  if (ei < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric second argument to `wordlist' function: '%s'.",
        e_str.c_str()));
  }

  const string&& text = args[2]->Eval(ev);
  int i = 0;
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    i++;
    if (si <= i && i <= ei) {
      ww.Write(tok);
    }
  }
}

void WordsFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  WordScanner ws(text);
  int n = 0;
  for (auto iter = ws.begin(); iter != ws.end(); ++iter)
    n++;
  char buf[32];
  sprintf(buf, "%d", n);
  *s += buf;
}

void FirstwordFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  for (StringPiece tok : WordScanner(text)) {
    AppendString(tok, s);
    return;
  }
}

void LastwordFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  StringPiece last;
  for (StringPiece tok : WordScanner(text)) {
    last = tok;
  }
  AppendString(last, s);
}

void JoinFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& list1 = args[0]->Eval(ev);
  const string&& list2 = args[1]->Eval(ev);
  WordScanner ws1(list1);
  WordScanner ws2(list2);
  WordWriter ww(s);
  WordScanner::Iterator iter1, iter2;
  for (iter1 = ws1.begin(), iter2 = ws2.begin();
       iter1 != ws1.end() && iter2 != ws2.end();
       ++iter1, ++iter2) {
    ww.Write(*iter1);
    // Use |AppendString| not to append extra ' '.
    AppendString(*iter2, s);
  }
  for (; iter1 != ws1.end(); ++iter1)
    ww.Write(*iter1);
  for (; iter2 != ws2.end(); ++iter2)
    ww.Write(*iter2);
}

void WildcardFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& pat = args[0]->Eval(ev);
  COLLECT_STATS("func wildcard time");
  // Note GNU make does not delay the execution of $(wildcard) so we
  // do not need to check avoid_io here.
  WordWriter ww(s);
  vector<string>* files;
  for (StringPiece tok : WordScanner(pat)) {
    ScopedTerminator st(tok);
    Glob(tok.data(), &files);
    for (const string& file : *files) {
      ww.Write(file);
    }
  }
}

void DirFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    ww.Write(Dirname(tok));
    s->push_back('/');
  }
}

void NotdirFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    if (tok == "/") {
      ww.Write(StringPiece(""));
    } else {
      ww.Write(Basename(tok));
    }
  }
}

void SuffixFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    StringPiece suf = GetExt(tok);
    if (!suf.empty())
      ww.Write(suf);
  }
}

void BasenameFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    ww.Write(StripExt(tok));
  }
}

void AddsuffixFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& suf = args[0]->Eval(ev);
  const string&& text = args[1]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    ww.Write(tok);
    *s += suf;
  }
}

void AddprefixFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& pre = args[0]->Eval(ev);
  const string&& text = args[1]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    ww.Write(pre);
    AppendString(tok, s);
  }
}

void RealpathFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "$(";
    string kati_binary;
    GetExecutablePath(&kati_binary);
    *s += kati_binary;
    *s += " --realpath ";
    *s += text;
    *s += " 2> /dev/null)";
    return;
  }

  WordWriter ww(s);
  for (StringPiece tok : WordScanner(text)) {
    ScopedTerminator st(tok);
    char buf[PATH_MAX];
    if (realpath(tok.data(), buf))
      ww.Write(buf);
  }
}

void AbspathFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  string buf;
  for (StringPiece tok : WordScanner(text)) {
    AbsPath(tok, &buf);
    ww.Write(buf);
  }
}

void IfFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& cond = args[0]->Eval(ev);
  if (cond.empty()) {
    if (args.size() > 2)
      args[2]->Eval(ev, s);
  } else {
    args[1]->Eval(ev, s);
  }
}

void AndFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  string cond;
  for (Value* a : args) {
    cond = a->Eval(ev);
    if (cond.empty())
      return;
  }
  if (!cond.empty()) {
    *s += cond;
  }
}

void OrFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  for (Value* a : args) {
    const string&& cond = a->Eval(ev);
    if (!cond.empty()) {
      *s += cond;
      return;
    }
  }
}

void ValueFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(Intern(var_name));
  AppendString(var->String().as_string(), s);
}

void EvalFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  // TODO: eval leaks everything... for now.
  //const string text = args[0]->Eval(ev);
  string* text = new string;
  args[0]->Eval(ev, text);
  if (ev->avoid_io()) {
    KATI_WARN("%s:%d: *warning*: $(eval) in a recipe is not recommended: %s",
              LOCF(ev->loc()), text->c_str());
  }
  vector<Stmt*> stmts;
  Parse(*text, ev->loc(), &stmts);
  for (Stmt* stmt : stmts) {
    LOG("%s", stmt->DebugString().c_str());
    stmt->Eval(ev);
    //delete stmt;
  }
}

//#define TEST_FIND_EMULATOR

// A hack for Android build. We need to evaluate things like $((3+4))
// when we emit ninja file, because the result of such expressions
// will be passed to other make functions.
// TODO: Maybe we should introduce a helper binary which evaluate
// make expressions at ninja-time.
static bool HasNoIoInShellScript(const string& cmd) {
  if (cmd.empty())
    return true;
  if (HasPrefix(cmd, "echo $((") && cmd[cmd.size()-1] == ')')
    return true;
  return false;
}

static void ShellFuncImpl(const string& shell, const string& cmd,
                          string* s, FindCommand** fc) {
  LOG("ShellFunc: %s", cmd.c_str());

#ifdef TEST_FIND_EMULATOR
  bool need_check = false;
  string out2;
#endif
  if (FindEmulator::Get()) {
    *fc = new FindCommand();
    if ((*fc)->Parse(cmd)) {
#ifdef TEST_FIND_EMULATOR
      if (FindEmulator::Get()->HandleFind(cmd, **fc, &out2)) {
        need_check = true;
      }
#else
      if (FindEmulator::Get()->HandleFind(cmd, **fc, s)) {
        return;
      }
#endif
    }
    delete *fc;
    *fc = NULL;
  }

  COLLECT_STATS_WITH_SLOW_REPORT("func shell time", cmd.c_str());
  RunCommand(shell, cmd, RedirectStderr::NONE, s);
  FormatForCommandSubstitution(s);

#ifdef TEST_FIND_EMULATOR
  if (need_check) {
    if (*s != out2) {
      ERROR("FindEmulator is broken: %s\n%s\nvs\n%s",
            cmd.c_str(), s->c_str(), out2.c_str());
    }
  }
#endif
}

static vector<CommandResult*> g_command_results;

bool ShouldStoreCommandResult(StringPiece cmd) {
  if (HasWord(cmd, "date") || HasWord(cmd, "echo"))
    return false;

  Pattern pat(g_flags.ignore_dirty_pattern);
  Pattern nopat(g_flags.no_ignore_dirty_pattern);
  for (StringPiece tok : WordScanner(cmd)) {
    if (pat.Match(tok) && !nopat.Match(tok)) {
      return false;
    }
  }

  return true;
}

void ShellFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  string cmd = args[0]->Eval(ev);
  if (ev->avoid_io() && !HasNoIoInShellScript(cmd)) {
    if (ev->eval_depth() > 1) {
      ERROR("%s:%d: kati doesn't support passing results of $(shell) "
            "to other make constructs: %s",
            LOCF(ev->loc()), cmd.c_str());
    }
    StripShellComment(&cmd);
    *s += "$(";
    *s += cmd;
    *s += ")";
    return;
  }

  const string&& shell = ev->GetShellAndFlag();

  string out;
  FindCommand* fc = NULL;
  ShellFuncImpl(shell, cmd, &out, &fc);
  if (ShouldStoreCommandResult(cmd)) {
    CommandResult* cr = new CommandResult();
    cr->shell = shell;
    cr->cmd = cmd;
    cr->find.reset(fc);
    cr->result = out;
    g_command_results.push_back(cr);
  }
  *s += out;
}

void CallFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  static const Symbol tmpvar_names[] = {
    Intern("0"), Intern("1"),  Intern("2"), Intern("3"), Intern("4"),
    Intern("5"), Intern("6"),  Intern("7"), Intern("8"), Intern("9")
  };

  const string&& func_name_buf = args[0]->Eval(ev);
  const StringPiece func_name = TrimSpace(func_name_buf);
  Var* func = ev->LookupVar(Intern(func_name));
  if (!func->IsDefined()) {
    KATI_WARN("%s:%d: *warning*: undefined user function: %s",
              ev->loc(), func_name.as_string().c_str());
  }
  vector<unique_ptr<SimpleVar>> av;
  for (size_t i = 1; i < args.size(); i++) {
    unique_ptr<SimpleVar> s(
        new SimpleVar(args[i]->Eval(ev), VarOrigin::AUTOMATIC));
    av.push_back(move(s));
  }
  vector<unique_ptr<ScopedGlobalVar>> sv;
  for (size_t i = 1; ; i++) {
    string s;
    Symbol tmpvar_name_sym(Symbol::IsUninitialized{});
    if (i < sizeof(tmpvar_names)/sizeof(tmpvar_names[0])) {
      tmpvar_name_sym = tmpvar_names[i];
    } else {
      s = StringPrintf("%d", i);
      tmpvar_name_sym = Intern(s);
    }
    if (i < args.size()) {
      sv.emplace_back(new ScopedGlobalVar(tmpvar_name_sym, av[i-1].get()));
    } else {
      // We need to blank further automatic vars
      Var *v = ev->LookupVar(tmpvar_name_sym);
      if (!v->IsDefined()) break;
      if (v->Origin() != VarOrigin::AUTOMATIC) break;

      av.emplace_back(new SimpleVar("", VarOrigin::AUTOMATIC));
      sv.emplace_back(new ScopedGlobalVar(tmpvar_name_sym, av[i-1].get()));
    }
  }

  ev->DecrementEvalDepth();
  func->Eval(ev, s);
  ev->IncrementEvalDepth();
}

void ForeachFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& varname = args[0]->Eval(ev);
  const string&& list = args[1]->Eval(ev);
  ev->DecrementEvalDepth();
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(list)) {
    unique_ptr<SimpleVar> v(new SimpleVar(
        tok.as_string(), VarOrigin::AUTOMATIC));
    ScopedGlobalVar sv(Intern(varname), v.get());
    ww.MaybeAddWhitespace();
    args[2]->Eval(ev, s);
  }
  ev->IncrementEvalDepth();
}

void OriginFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(Intern(var_name));
  *s += GetOriginStr(var->Origin());
}

void FlavorFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  const string&& var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(Intern(var_name));
  *s += var->Flavor();
}

void InfoFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  const string&& a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    ev->add_delayed_output_command(StringPrintf("echo -e \"%s\"", EchoEscape(a).c_str()));
    return;
  }
  printf("%s\n", a.c_str());
  fflush(stdout);
}

void WarningFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  const string&& a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    ev->add_delayed_output_command(
        StringPrintf("echo -e \"%s:%d: %s\" 2>&1", LOCF(ev->loc()), EchoEscape(a).c_str()));
    return;
  }
  printf("%s:%d: %s\n", LOCF(ev->loc()), a.c_str());
  fflush(stdout);
}

void ErrorFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  const string&& a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    ev->add_delayed_output_command(
        StringPrintf("echo -e \"%s:%d: *** %s.\" 2>&1 && false",
                     LOCF(ev->loc()), EchoEscape(a).c_str()));
    return;
  }
  ev->Error(StringPrintf("*** %s.", a.c_str()));
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
    bool ok = g_func_info_map->emplace(fi->name, fi).second;
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

const vector<CommandResult*>& GetShellCommandResults() {
  return g_command_results;
}
