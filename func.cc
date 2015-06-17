#include "func.h"

#include <glob.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>

#include <algorithm>
#include <iterator>
#include <unordered_map>

#include "eval.h"
#include "log.h"
#include "strutil.h"

namespace {

void PatsubstFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pat = args[0]->Eval(ev);
  shared_ptr<string> repl = args[1]->Eval(ev);
  shared_ptr<string> str = args[2]->Eval(ev);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*str)) {
    ww.MaybeAddWhitespace();
    AppendSubstPattern(tok, *pat, *repl, s);
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
  vector<StringPiece> pats;
  WordScanner(*pat_buf).Split(&pats);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    for (StringPiece pat : pats) {
      if (MatchPattern(tok, pat)) {
        ww.Write(tok);
        break;
      }
    }
  }
}

void FilterOutFunc(const vector<Value*>& args, Evaluator* ev, string* s) {
  shared_ptr<string> pat_buf = args[0]->Eval(ev);
  shared_ptr<string> text = args[1]->Eval(ev);
  vector<StringPiece> pats;
  WordScanner(*pat_buf).Split(&pats);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(*text)) {
    bool matched = false;
    for (StringPiece pat : pats) {
      if (MatchPattern(tok, pat)) {
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

void WildcardFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(wildcard)");
}

void DirFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(dir)");
}

void NotdirFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(notdir)");
}

void SuffixFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(suffix)");
}

void BasenameFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(basename)");
}

void AddsuffixFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(addsuffix)");
}

void AddprefixFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(addprefix)");
}

void RealpathFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(realpath)");
}

void AbspathFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(abspath)");
}

void IfFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(if)");
}

void AndFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(and)");
}

void OrFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(or)");
}

void ValueFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(value)");
}

void EvalFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(eval)");
}

void ShellFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(shell)");
}

void CallFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(call)");
}

void ForeachFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(foreach)");
}

void OriginFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(origin)");
}

void FlavorFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(flavor)");
}

void InfoFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  printf("%s\n", a->c_str());
  fflush(stdout);
}

void WarningFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  printf("%s:%d: %s\n", LOCF(ev->loc()), a->c_str());
  fflush(stdout);
}

void ErrorFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  ev->Error(StringPrintf("*** %s.", a->c_str()));
}

FuncInfo g_func_infos[] = {
  { "patsubst", &PatsubstFunc, 3 },
  { "strip", &StripFunc, 1 },
  { "subst", &SubstFunc, 3 },
  { "findstring", &FindstringFunc, 2 },
  { "filter", &FilterFunc, 2 },
  { "filter-out", &FilterOutFunc, 2 },
  { "sort", &SortFunc, 1 },
  { "word", &WordFunc, 2 },
  { "wordlist", &WordlistFunc, 3 },
  { "words", &WordsFunc, 1 },
  { "firstword", &FirstwordFunc, 1 },
  { "lastword", &LastwordFunc, 1 },

  { "join", &JoinFunc, 2 },
  { "wildcard", &WildcardFunc, 1 },
  { "dir", &DirFunc, 1 },
  { "notdir", &NotdirFunc, 1 },
  { "suffix", &SuffixFunc, 1 },
  { "basename", &BasenameFunc, 1 },
  { "addsuffix", &AddsuffixFunc, 2 },
  { "addprefix", &AddprefixFunc, 2 },
  { "realpath", &RealpathFunc, 1 },
  { "abspath", &AbspathFunc, 1 },

  { "if", &IfFunc, 1 },
  { "and", &AndFunc, 1 },
  { "or", &OrFunc, 1 },
  { "value", &ValueFunc, 1 },
  { "eval", &EvalFunc, 1 },
  { "shell", &ShellFunc, 1 },
  { "call", &CallFunc, 1 },
  { "foreach", &ForeachFunc, 1 },
  { "origin", &OriginFunc, 1 },
  { "flavor", &FlavorFunc, 1 },
  { "info", &InfoFunc, 1 },
  { "warning", &WarningFunc, 1 },
  { "error", &ErrorFunc, 1 },
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
