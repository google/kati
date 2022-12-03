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
#include <fcntl.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/stat.h>
#include <unistd.h>

#include <algorithm>
#include <iterator>
#include <memory>
#include <unordered_map>

#include "eval.h"
#include "fileutil.h"
#include "find.h"
#include "loc.h"
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
void StripShellComment(std::string* cmd) {
  if (cmd->find('#') == std::string::npos)
    return;

  std::string res;
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
        [[fallthrough]];

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

void PatsubstFunc(const std::vector<Value*>& args,
                  Evaluator* ev,
                  std::string* s) {
  const std::string&& pat_str = args[0]->Eval(ev);
  const std::string&& repl = args[1]->Eval(ev);
  const std::string&& str = args[2]->Eval(ev);
  WordWriter ww(s);
  Pattern pat(pat_str);
  for (std::string_view tok : WordScanner(str)) {
    ww.MaybeAddWhitespace();
    pat.AppendSubst(tok, repl, s);
  }
}

void StripFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& str = args[0]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(str)) {
    ww.Write(tok);
  }
}

void SubstFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& pat = args[0]->Eval(ev);
  const std::string&& repl = args[1]->Eval(ev);
  const std::string&& str = args[2]->Eval(ev);
  if (pat.empty()) {
    *s += str;
    *s += repl;
    return;
  }
  size_t index = 0;
  while (index < str.size()) {
    size_t found = str.find(pat, index);
    if (found == std::string::npos)
      break;
    AppendString(std::string_view(str).substr(index, found - index), s);
    AppendString(repl, s);
    index = found + pat.size();
  }
  AppendString(std::string_view(str).substr(index), s);
}

void FindstringFunc(const std::vector<Value*>& args,
                    Evaluator* ev,
                    std::string* s) {
  const std::string&& find = args[0]->Eval(ev);
  const std::string&& in = args[1]->Eval(ev);
  if (in.find(find) != std::string::npos)
    AppendString(find, s);
}

void FilterFunc(const std::vector<Value*>& args,
                Evaluator* ev,
                std::string* s) {
  const std::string&& pat_buf = args[0]->Eval(ev);
  const std::string&& text = args[1]->Eval(ev);
  std::vector<Pattern> pats;
  for (std::string_view pat : WordScanner(pat_buf)) {
    pats.push_back(Pattern(pat));
  }
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    for (const Pattern& pat : pats) {
      if (pat.Match(tok)) {
        ww.Write(tok);
        break;
      }
    }
  }
}

void FilterOutFunc(const std::vector<Value*>& args,
                   Evaluator* ev,
                   std::string* s) {
  const std::string&& pat_buf = args[0]->Eval(ev);
  const std::string&& text = args[1]->Eval(ev);
  std::vector<Pattern> pats;
  for (std::string_view pat : WordScanner(pat_buf)) {
    pats.push_back(Pattern(pat));
  }
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
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

void SortFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  std::string list;
  args[0]->Eval(ev, &list);
  COLLECT_STATS("func sort time");
  // TODO(hamaji): Probably we could use a faster string-specific sort
  // algorithm.
  std::vector<std::string_view> toks;
  WordScanner(list).Split(&toks);
  stable_sort(toks.begin(), toks.end());
  WordWriter ww(s);
  std::string_view prev;
  for (std::string_view tok : toks) {
    if (prev != tok) {
      ww.Write(tok);
      prev = tok;
    }
  }
}

static int GetNumericValueForFunc(const std::string& buf) {
  std::string_view s = TrimLeftSpace(buf);
  char* end;
  long n = strtol(s.data(), &end, 10);
  if (n < 0 || n == LONG_MAX || s.data() + s.size() != end) {
    return -1;
  }
  return n;
}

void WordFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& n_str = args[0]->Eval(ev);
  int n = GetNumericValueForFunc(n_str);
  if (n < 0) {
    ev->Error(
        StringPrintf("*** non-numeric first argument to `word' function: '%s'.",
                     n_str.c_str()));
  }
  if (n == 0) {
    ev->Error("*** first argument to `word' function must be greater than 0.");
  }

  const std::string&& text = args[1]->Eval(ev);
  for (std::string_view tok : WordScanner(text)) {
    n--;
    if (n == 0) {
      AppendString(tok, s);
      break;
    }
  }
}

void WordlistFunc(const std::vector<Value*>& args,
                  Evaluator* ev,
                  std::string* s) {
  const std::string&& s_str = args[0]->Eval(ev);
  int si = GetNumericValueForFunc(s_str);
  if (si < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric first argument to `wordlist' function: '%s'.",
        s_str.c_str()));
  }
  if (si == 0) {
    ev->Error(
        StringPrintf("*** invalid first argument to `wordlist' function: %s`",
                     s_str.c_str()));
  }

  const std::string&& e_str = args[1]->Eval(ev);
  int ei = GetNumericValueForFunc(e_str);
  if (ei < 0) {
    ev->Error(StringPrintf(
        "*** non-numeric second argument to `wordlist' function: '%s'.",
        e_str.c_str()));
  }

  const std::string&& text = args[2]->Eval(ev);
  int i = 0;
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    i++;
    if (si <= i && i <= ei) {
      ww.Write(tok);
    }
  }
}

void WordsFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  WordScanner ws(text);
  int n = 0;
  for (auto iter = ws.begin(); iter != ws.end(); ++iter)
    n++;
  char buf[32];
  sprintf(buf, "%d", n);
  *s += buf;
}

void FirstwordFunc(const std::vector<Value*>& args,
                   Evaluator* ev,
                   std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  WordScanner ws(text);
  auto begin = ws.begin();
  if (begin != ws.end()) {
    AppendString(*begin, s);
  }
}

void LastwordFunc(const std::vector<Value*>& args,
                  Evaluator* ev,
                  std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  std::string_view last;
  for (std::string_view tok : WordScanner(text)) {
    last = tok;
  }
  AppendString(last, s);
}

void JoinFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& list1 = args[0]->Eval(ev);
  const std::string&& list2 = args[1]->Eval(ev);
  WordScanner ws1(list1);
  WordScanner ws2(list2);
  WordWriter ww(s);
  WordScanner::Iterator iter1, iter2;
  for (iter1 = ws1.begin(), iter2 = ws2.begin();
       iter1 != ws1.end() && iter2 != ws2.end(); ++iter1, ++iter2) {
    ww.Write(*iter1);
    // Use |AppendString| not to append extra ' '.
    AppendString(*iter2, s);
  }
  for (; iter1 != ws1.end(); ++iter1)
    ww.Write(*iter1);
  for (; iter2 != ws2.end(); ++iter2)
    ww.Write(*iter2);
}

void WildcardFunc(const std::vector<Value*>& args,
                  Evaluator* ev,
                  std::string* s) {
  const std::string&& pat = args[0]->Eval(ev);
  COLLECT_STATS("func wildcard time");
  // Note GNU make does not delay the execution of $(wildcard) so we
  // do not need to check avoid_io here.
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(pat)) {
    ScopedTerminator st(tok);
    const auto& files = Glob(tok.data());
    for (const std::string& file : files) {
      ww.Write(file);
    }
  }
}

void DirFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    ww.Write(Dirname(tok));
    s->push_back('/');
  }
}

void NotdirFunc(const std::vector<Value*>& args,
                Evaluator* ev,
                std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    if (tok == "/") {
      ww.Write(std::string_view(""));
    } else {
      ww.Write(Basename(tok));
    }
  }
}

void SuffixFunc(const std::vector<Value*>& args,
                Evaluator* ev,
                std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    std::string_view suf = GetExt(tok);
    if (!suf.empty())
      ww.Write(suf);
  }
}

void BasenameFunc(const std::vector<Value*>& args,
                  Evaluator* ev,
                  std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    ww.Write(StripExt(tok));
  }
}

void AddsuffixFunc(const std::vector<Value*>& args,
                   Evaluator* ev,
                   std::string* s) {
  const std::string&& suf = args[0]->Eval(ev);
  const std::string&& text = args[1]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    ww.Write(tok);
    *s += suf;
  }
}

void AddprefixFunc(const std::vector<Value*>& args,
                   Evaluator* ev,
                   std::string* s) {
  const std::string&& pre = args[0]->Eval(ev);
  const std::string&& text = args[1]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    ww.Write(pre);
    AppendString(tok, s);
  }
}

void RealpathFunc(const std::vector<Value*>& args,
                  Evaluator* ev,
                  std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    *s += "$(";
    *s += GetExecutablePath();
    *s += " --realpath ";
    *s += text;
    *s += " 2> /dev/null)";
    return;
  }

  WordWriter ww(s);
  for (std::string_view tok : WordScanner(text)) {
    ScopedTerminator st(tok);
    char buf[PATH_MAX];
    if (realpath(tok.data(), buf))
      ww.Write(buf);
  }
}

void AbspathFunc(const std::vector<Value*>& args,
                 Evaluator* ev,
                 std::string* s) {
  const std::string&& text = args[0]->Eval(ev);
  WordWriter ww(s);
  std::string buf;
  for (std::string_view tok : WordScanner(text)) {
    AbsPath(tok, &buf);
    ww.Write(buf);
  }
}

void IfFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& cond = args[0]->Eval(ev);
  if (cond.empty()) {
    if (args.size() > 2)
      args[2]->Eval(ev, s);
  } else {
    args[1]->Eval(ev, s);
  }
}

void AndFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  std::string cond;
  for (Value* a : args) {
    cond = a->Eval(ev);
    if (cond.empty())
      return;
  }
  if (!cond.empty()) {
    *s += cond;
  }
}

void OrFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  for (Value* a : args) {
    const std::string&& cond = a->Eval(ev);
    if (!cond.empty()) {
      *s += cond;
      return;
    }
  }
}

void ValueFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  const std::string&& var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(Intern(var_name));
  AppendString(std::string(var->String()), s);
}

void EvalFunc(const std::vector<Value*>& args, Evaluator* ev, std::string*) {
  // TODO: eval leaks everything... for now.
  // const string text = args[0]->Eval(ev);
  ev->CheckStack();
  std::string* text = new std::string;
  args[0]->Eval(ev, text);
  if (ev->avoid_io()) {
    KATI_WARN_LOC(ev->loc(),
                  "*warning*: $(eval) in a recipe is not recommended: %s",
                  text->c_str());
  }
  std::vector<Stmt*> stmts;
  Parse(*text, ev->loc(), &stmts);
  for (Stmt* stmt : stmts) {
    LOG("%s", stmt->DebugString().c_str());
    stmt->Eval(ev);
    // delete stmt;
  }
}

//#define TEST_FIND_EMULATOR

// A hack for Android build. We need to evaluate things like $((3+4))
// when we emit ninja file, because the result of such expressions
// will be passed to other make functions.
// TODO: Maybe we should introduce a helper binary which evaluate
// make expressions at ninja-time.
static bool HasNoIoInShellScript(const std::string& cmd) {
  if (cmd.empty())
    return true;
  if (HasPrefix(cmd, "echo $((") && cmd[cmd.size() - 1] == ')')
    return true;
  return false;
}

static int ShellFuncImpl(const std::string& shell,
                         const std::string& shellflag,
                         const std::string& cmd,
                         const Loc& loc,
                         std::string* s,
                         FindCommand** fc) {
  LOG("ShellFunc: %s", cmd.c_str());

#ifdef TEST_FIND_EMULATOR
  bool need_check = false;
  string out2;
#endif
  if (FindEmulator::Get()) {
    *fc = new FindCommand();
    if ((*fc)->Parse(cmd)) {
#ifdef TEST_FIND_EMULATOR
      if (FindEmulator::Get()->HandleFind(cmd, **fc, loc, &out2)) {
        need_check = true;
      }
#else
      if (FindEmulator::Get()->HandleFind(cmd, **fc, loc, s)) {
        return 0;
      }
#endif
    }
    delete *fc;
    *fc = NULL;
  }

  COLLECT_STATS_WITH_SLOW_REPORT("func shell time", cmd.c_str());
  int status = RunCommand(shell, shellflag, cmd, RedirectStderr::NONE, s);
  FormatForCommandSubstitution(s);

#ifdef TEST_FIND_EMULATOR
  if (need_check) {
    if (*s != out2) {
      ERROR("FindEmulator is broken: %s\n%s\nvs\n%s", cmd.c_str(), s->c_str(),
            out2.c_str());
    }
  }
#endif

  if (WIFEXITED(status)) {
    return WEXITSTATUS(status);
  }
  return 1;
}

static std::vector<CommandResult*> g_command_results;

bool ShouldStoreCommandResult(std::string_view cmd) {
  // We really just want to ignore this one, or remove BUILD_DATETIME from
  // Android completely
  if (cmd == "date +%s")
    return false;

  Pattern pat(g_flags.ignore_dirty_pattern ? g_flags.ignore_dirty_pattern : "");
  Pattern nopat(
      g_flags.no_ignore_dirty_pattern ? g_flags.no_ignore_dirty_pattern : "");
  for (std::string_view tok : WordScanner(cmd)) {
    if (pat.Match(tok) && !nopat.Match(tok)) {
      return false;
    }
  }

  return true;
}

void ShellFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  std::string cmd = args[0]->Eval(ev);
  if (ev->avoid_io() && !HasNoIoInShellScript(cmd)) {
    if (ev->eval_depth() > 1) {
      ERROR_LOC(ev->loc(),
                "kati doesn't support passing results of $(shell) "
                "to other make constructs: %s",
                cmd.c_str());
    }
    StripShellComment(&cmd);
    *s += "$(";
    *s += cmd;
    *s += ")";
    return;
  }

  const std::string&& shell = ev->GetShell();
  const std::string&& shellflag = ev->GetShellFlag();

  std::string out;
  FindCommand* fc = NULL;
  int returnCode = ShellFuncImpl(shell, shellflag, cmd, ev->loc(), &out, &fc);
  if (ShouldStoreCommandResult(cmd)) {
    CommandResult* cr = new CommandResult();
    cr->op = (fc == NULL) ? CommandOp::SHELL : CommandOp::FIND,
    cr->shell = shell;
    cr->shellflag = shellflag;
    cr->cmd = cmd;
    cr->find.reset(fc);
    cr->result = out;
    cr->loc = ev->loc();
    g_command_results.push_back(cr);
  }
  *s += out;
  ShellStatusVar::SetValue(returnCode);
}

void CallFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  static const Symbol tmpvar_names[] = {
      Intern("0"), Intern("1"), Intern("2"), Intern("3"), Intern("4"),
      Intern("5"), Intern("6"), Intern("7"), Intern("8"), Intern("9")};

  ev->CheckStack();
  const std::string&& func_name_buf = args[0]->Eval(ev);
  Symbol func_sym = Intern(TrimSpace(func_name_buf));
  Var* func = ev->LookupVar(func_sym);
  func->Used(ev, func_sym);
  if (!func->IsDefined()) {
    KATI_WARN_LOC(ev->loc(), "*warning*: undefined user function: %s",
                  func_sym.c_str());
  }
  std::vector<std::unique_ptr<SimpleVar>> av;
  for (size_t i = 1; i < args.size(); i++) {
    av.emplace_back(std::make_unique<SimpleVar>(
        args[i]->Eval(ev), VarOrigin::AUTOMATIC, nullptr, Loc()));
  }
  std::vector<std::unique_ptr<ScopedGlobalVar>> sv;
  for (size_t i = 1;; i++) {
    std::string s;
    Symbol tmpvar_name_sym;
    if (i < sizeof(tmpvar_names) / sizeof(tmpvar_names[0])) {
      tmpvar_name_sym = tmpvar_names[i];
    } else {
      s = StringPrintf("%d", i);
      tmpvar_name_sym = Intern(s);
    }
    if (i < args.size()) {
      sv.emplace_back(new ScopedGlobalVar(tmpvar_name_sym, av[i - 1].get()));
    } else {
      // We need to blank further automatic vars
      Var* v = ev->LookupVar(tmpvar_name_sym);
      if (!v->IsDefined())
        break;
      if (v->Origin() != VarOrigin::AUTOMATIC)
        break;

      av.emplace_back(new SimpleVar("", VarOrigin::AUTOMATIC, nullptr, Loc()));
      sv.emplace_back(new ScopedGlobalVar(tmpvar_name_sym, av[i - 1].get()));
    }
  }

  ev->DecrementEvalDepth();

  {
    ScopedFrame frame(ev->Enter(FrameType::CALL, func_sym.str(), ev->loc()));
    func->Eval(ev, s);
  }

  ev->IncrementEvalDepth();
}

void ForeachFunc(const std::vector<Value*>& args,
                 Evaluator* ev,
                 std::string* s) {
  const std::string&& varname = args[0]->Eval(ev);
  const std::string&& list = args[1]->Eval(ev);
  ev->DecrementEvalDepth();
  WordWriter ww(s);
  for (std::string_view tok : WordScanner(list)) {
    std::unique_ptr<SimpleVar> v(
        new SimpleVar(std::string(tok), VarOrigin::AUTOMATIC, nullptr, Loc()));
    ScopedGlobalVar sv(Intern(varname), v.get());
    ww.MaybeAddWhitespace();
    args[2]->Eval(ev, s);
  }
  ev->IncrementEvalDepth();
}

void OriginFunc(const std::vector<Value*>& args,
                Evaluator* ev,
                std::string* s) {
  const std::string&& var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(Intern(var_name));
  *s += GetOriginStr(var->Origin());
}

void FlavorFunc(const std::vector<Value*>& args,
                Evaluator* ev,
                std::string* s) {
  const std::string&& var_name = args[0]->Eval(ev);
  Var* var = ev->LookupVar(Intern(var_name));
  *s += var->Flavor();
}

void InfoFunc(const std::vector<Value*>& args, Evaluator* ev, std::string*) {
  const std::string&& a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    ev->add_delayed_output_command(
        StringPrintf("echo -e \"%s\"", EchoEscape(a).c_str()));
    return;
  }
  printf("%s\n", a.c_str());
  fflush(stdout);
}

void WarningFunc(const std::vector<Value*>& args, Evaluator* ev, std::string*) {
  const std::string&& a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    ev->add_delayed_output_command(StringPrintf(
        "echo -e \"%s:%d: %s\" 2>&1", LOCF(ev->loc()), EchoEscape(a).c_str()));
    return;
  }
  WARN_LOC(ev->loc(), "%s", a.c_str());
}

void ErrorFunc(const std::vector<Value*>& args, Evaluator* ev, std::string*) {
  const std::string&& a = args[0]->Eval(ev);
  if (ev->avoid_io()) {
    ev->add_delayed_output_command(
        StringPrintf("echo -e \"%s:%d: *** %s.\" 2>&1 && false",
                     LOCF(ev->loc()), EchoEscape(a).c_str()));
    return;
  }
  ev->Error(StringPrintf("*** %s.", a.c_str()));
}

static void FileReadFunc(Evaluator* ev,
                         const std::string& filename,
                         std::string* s) {
  int fd = open(filename.c_str(), O_RDONLY);
  if (fd < 0) {
    if (errno == ENOENT) {
      if (ShouldStoreCommandResult(filename)) {
        CommandResult* cr = new CommandResult();
        cr->op = CommandOp::READ_MISSING;
        cr->cmd = filename;
        cr->loc = ev->loc();
        g_command_results.push_back(cr);
      }
      return;
    } else {
      ev->Error("*** open failed.");
    }
  }

  struct stat st;
  if (fstat(fd, &st) < 0) {
    ev->Error("*** fstat failed.");
  }

  size_t len = st.st_size;
  std::string out;
  out.resize(len);
  ssize_t r = HANDLE_EINTR(read(fd, &out[0], len));
  if (r != static_cast<ssize_t>(len)) {
    ev->Error("*** read failed.");
  }

  if (close(fd) < 0) {
    ev->Error("*** close failed.");
  }

  if (out.back() == '\n') {
    out.pop_back();
  }

  if (ShouldStoreCommandResult(filename)) {
    CommandResult* cr = new CommandResult();
    cr->op = CommandOp::READ;
    cr->cmd = filename;
    cr->loc = ev->loc();
    g_command_results.push_back(cr);
  }
  *s += out;
}

static void FileWriteFunc(Evaluator* ev,
                          const std::string& filename,
                          bool append,
                          std::string text) {
  FILE* f = fopen(filename.c_str(), append ? "ab" : "wb");
  if (f == NULL) {
    ev->Error("*** fopen failed.");
  }

  if (fwrite(&text[0], text.size(), 1, f) != 1) {
    ev->Error("*** fwrite failed.");
  }

  if (fclose(f) != 0) {
    ev->Error("*** fclose failed.");
  }

  if (ShouldStoreCommandResult(filename)) {
    CommandResult* cr = new CommandResult();
    cr->op = CommandOp::WRITE;
    cr->cmd = filename;
    cr->result = text;
    cr->loc = ev->loc();
    g_command_results.push_back(cr);
  }
}

void FileFunc(const std::vector<Value*>& args, Evaluator* ev, std::string* s) {
  if (ev->avoid_io()) {
    ev->Error("*** $(file ...) is not supported in rules.");
  }

  std::string arg = args[0]->Eval(ev);
  std::string_view filename = TrimSpace(arg);

  if (filename.size() <= 1) {
    ev->Error("*** Missing filename");
  }

  if (filename[0] == '<') {
    filename = TrimLeftSpace(filename.substr(1));
    if (!filename.size()) {
      ev->Error("*** Missing filename");
    }
    if (args.size() > 1) {
      ev->Error("*** invalid argument");
    }

    FileReadFunc(ev, std::string(filename), s);
  } else if (filename[0] == '>') {
    bool append = false;
    if (filename[1] == '>') {
      append = true;
      filename = filename.substr(2);
    } else {
      filename = filename.substr(1);
    }
    filename = TrimLeftSpace(filename);
    if (!filename.size()) {
      ev->Error("*** Missing filename");
    }

    std::string text;
    if (args.size() > 1) {
      text = args[1]->Eval(ev);
      if (text.size() == 0 || text.back() != '\n') {
        text.push_back('\n');
      }
    }

    FileWriteFunc(ev, std::string(filename), append, text);
  } else {
    ev->Error(StringPrintf("*** Invalid file operation: %s.  Stop.",
                           std::string(filename).c_str()));
  }
}

void DeprecatedVarFunc(const std::vector<Value*>& args,
                       Evaluator* ev,
                       std::string*) {
  std::string vars_str = args[0]->Eval(ev);
  std::string msg;

  if (args.size() == 2) {
    msg = ". " + args[1]->Eval(ev);
  }

  if (ev->avoid_io()) {
    ev->Error("*** $(KATI_deprecated_var ...) is not supported in rules.");
  }

  for (std::string_view var : WordScanner(vars_str)) {
    Symbol sym = Intern(var);
    Var* v = ev->PeekVar(sym);
    if (!v->IsDefined()) {
      v = new SimpleVar(VarOrigin::FILE, ev->CurrentFrame(), ev->loc());
      sym.SetGlobalVar(v, false, nullptr);
    }

    if (v->Deprecated()) {
      ev->Error(
          StringPrintf("*** Cannot call KATI_deprecated_var on already "
                       "deprecated variable: %s.",
                       sym.c_str()));
    } else if (v->Obsolete()) {
      ev->Error(
          StringPrintf("*** Cannot call KATI_deprecated_var on already "
                       "obsolete variable: %s.",
                       sym.c_str()));
    }

    v->SetDeprecated(msg);
  }
}

void ObsoleteVarFunc(const std::vector<Value*>& args,
                     Evaluator* ev,
                     std::string*) {
  std::string vars_str = args[0]->Eval(ev);
  std::string msg;

  if (args.size() == 2) {
    msg = ". " + args[1]->Eval(ev);
  }

  if (ev->avoid_io()) {
    ev->Error("*** $(KATI_obsolete_var ...) is not supported in rules.");
  }

  for (std::string_view var : WordScanner(vars_str)) {
    Symbol sym = Intern(var);
    Var* v = ev->PeekVar(sym);
    if (!v->IsDefined()) {
      v = new SimpleVar(VarOrigin::FILE, ev->CurrentFrame(), ev->loc());
      sym.SetGlobalVar(v, false, nullptr);
    }

    if (v->Deprecated()) {
      ev->Error(
          StringPrintf("*** Cannot call KATI_obsolete_var on already "
                       "deprecated variable: %s.",
                       sym.c_str()));
    } else if (v->Obsolete()) {
      ev->Error(StringPrintf(
          "*** Cannot call KATI_obsolete_var on already obsolete variable: %s.",
          sym.c_str()));
    }

    v->SetObsolete(msg);
  }
}

void DeprecateExportFunc(const std::vector<Value*>& args,
                         Evaluator* ev,
                         std::string*) {
  std::string msg = ". " + args[0]->Eval(ev);

  if (ev->avoid_io()) {
    ev->Error("*** $(KATI_deprecate_export) is not supported in rules.");
  }

  if (ev->ExportObsolete()) {
    ev->Error("*** Export is already obsolete.");
  } else if (ev->ExportDeprecated()) {
    ev->Error("*** Export is already deprecated.");
  }

  ev->SetExportDeprecated(msg);
}

void ObsoleteExportFunc(const std::vector<Value*>& args,
                        Evaluator* ev,
                        std::string*) {
  std::string msg = ". " + args[0]->Eval(ev);

  if (ev->avoid_io()) {
    ev->Error("*** $(KATI_obsolete_export) is not supported in rules.");
  }

  if (ev->ExportObsolete()) {
    ev->Error("*** Export is already obsolete.");
  }

  ev->SetExportObsolete(msg);
}

void ProfileFunc(const std::vector<Value*>& args, Evaluator* ev, std::string*) {
  for (auto arg : args) {
    std::string files = arg->Eval(ev);
    for (std::string_view file : WordScanner(files)) {
      ev->ProfileMakefile(file);
    }
  }
}

void VariableLocationFunc(const std::vector<Value*>& args,
                          Evaluator* ev,
                          std::string* s) {
  std::string arg = args[0]->Eval(ev);
  WordWriter ww(s);
  for (std::string_view var : WordScanner(arg)) {
    Symbol sym = Intern(var);
    Var* v = ev->PeekVar(sym);
    const Loc& loc = v->Location();
    ww.Write(loc.filename ? loc.filename : "<unknown>");
    AppendString(":", s);
    AppendString(std::to_string(loc.lineno > 0 ? loc.lineno : 0), s);
  }
}

#define ENTRY(name, args...) \
  {                          \
    name, { name, args }     \
  }

static const std::unordered_map<std::string_view, FuncInfo> g_func_info_map = {

    ENTRY("patsubst", &PatsubstFunc, 3, 3, false, false),
    ENTRY("strip", &StripFunc, 1, 1, false, false),
    ENTRY("subst", &SubstFunc, 3, 3, false, false),
    ENTRY("findstring", &FindstringFunc, 2, 2, false, false),
    ENTRY("filter", &FilterFunc, 2, 2, false, false),
    ENTRY("filter-out", &FilterOutFunc, 2, 2, false, false),
    ENTRY("sort", &SortFunc, 1, 1, false, false),
    ENTRY("word", &WordFunc, 2, 2, false, false),
    ENTRY("wordlist", &WordlistFunc, 3, 3, false, false),
    ENTRY("words", &WordsFunc, 1, 1, false, false),
    ENTRY("firstword", &FirstwordFunc, 1, 1, false, false),
    ENTRY("lastword", &LastwordFunc, 1, 1, false, false),

    ENTRY("join", &JoinFunc, 2, 2, false, false),
    ENTRY("wildcard", &WildcardFunc, 1, 1, false, false),
    ENTRY("dir", &DirFunc, 1, 1, false, false),
    ENTRY("notdir", &NotdirFunc, 1, 1, false, false),
    ENTRY("suffix", &SuffixFunc, 1, 1, false, false),
    ENTRY("basename", &BasenameFunc, 1, 1, false, false),
    ENTRY("addsuffix", &AddsuffixFunc, 2, 2, false, false),
    ENTRY("addprefix", &AddprefixFunc, 2, 2, false, false),
    ENTRY("realpath", &RealpathFunc, 1, 1, false, false),
    ENTRY("abspath", &AbspathFunc, 1, 1, false, false),

    ENTRY("if", &IfFunc, 3, 2, false, true),
    ENTRY("and", &AndFunc, 0, 0, true, false),
    ENTRY("or", &OrFunc, 0, 0, true, false),

    ENTRY("value", &ValueFunc, 1, 1, false, false),
    ENTRY("eval", &EvalFunc, 1, 1, false, false),
    ENTRY("shell", &ShellFunc, 1, 1, false, false),
    ENTRY("call", &CallFunc, 0, 0, false, false),
    ENTRY("foreach", &ForeachFunc, 3, 3, false, false),

    ENTRY("origin", &OriginFunc, 1, 1, false, false),
    ENTRY("flavor", &FlavorFunc, 1, 1, false, false),

    ENTRY("info", &InfoFunc, 1, 1, false, false),
    ENTRY("warning", &WarningFunc, 1, 1, false, false),
    ENTRY("error", &ErrorFunc, 1, 1, false, false),

    ENTRY("file", &FileFunc, 2, 1, false, false),

    /* Kati custom extension functions */
    ENTRY("KATI_deprecated_var", &DeprecatedVarFunc, 2, 1, false, false),
    ENTRY("KATI_obsolete_var", &ObsoleteVarFunc, 2, 1, false, false),
    ENTRY("KATI_deprecate_export", &DeprecateExportFunc, 1, 1, false, false),
    ENTRY("KATI_obsolete_export", &ObsoleteExportFunc, 1, 1, false, false),

    ENTRY("KATI_profile_makefile", &ProfileFunc, 0, 0, false, false),
    ENTRY("KATI_variable_location", &VariableLocationFunc, 1, 1, false, false),
};

}  // namespace

const FuncInfo* GetFuncInfo(std::string_view name) {
  auto found = g_func_info_map.find(name);
  if (found == g_func_info_map.end())
    return nullptr;
  return &found->second;
}

const std::vector<CommandResult*>& GetShellCommandResults() {
  return g_command_results;
}
