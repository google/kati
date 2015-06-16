#include "func.h"

#include <stdio.h>

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

void FindstringFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(findstring)");
}

void FilterFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(filter)");
}

void FilterOutFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(filter-out)");
}

void SortFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(sort)");
}

void WordFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(word)");
}

void WordlistFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(wordlist)");
}

void WordsFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(words)");
}

void FirstwordFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(firstword)");
}

void LastwordFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(lastword)");
}

void JoinFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(join)");
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
  { "addsuffix", &AddsuffixFunc, 1 },
  { "addprefix", &AddprefixFunc, 1 },
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
