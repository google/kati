#include "func.h"

#include <stdio.h>

#include <unordered_map>

#include "eval.h"
#include "log.h"
#include "strutil.h"

namespace {

void BuiltinPatsubstFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(patsubst)");
}

void BuiltinStripFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(strip)");
}

void BuiltinSubstFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(subst)");
}

void BuiltinFindstringFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(findstring)");
}

void BuiltinFilterFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(filter)");
}

void BuiltinFilterOutFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(filter-out)");
}

void BuiltinSortFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(sort)");
}

void BuiltinWordFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(word)");
}

void BuiltinWordlistFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(wordlist)");
}

void BuiltinWordsFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(words)");
}

void BuiltinFirstwordFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(firstword)");
}

void BuiltinLastwordFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(lastword)");
}

void BuiltinJoinFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(join)");
}

void BuiltinWildcardFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(wildcard)");
}

void BuiltinDirFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(dir)");
}

void BuiltinNotdirFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(notdir)");
}

void BuiltinSuffixFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(suffix)");
}

void BuiltinBasenameFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(basename)");
}

void BuiltinAddsuffixFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(addsuffix)");
}

void BuiltinAddprefixFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(addprefix)");
}

void BuiltinRealpathFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(realpath)");
}

void BuiltinAbspathFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(abspath)");
}

void BuiltinIfFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(if)");
}

void BuiltinAndFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(and)");
}

void BuiltinOrFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(or)");
}

void BuiltinValueFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(value)");
}

void BuiltinEvalFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(eval)");
}

void BuiltinShellFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(shell)");
}

void BuiltinCallFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(call)");
}

void BuiltinForeachFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(foreach)");
}

void BuiltinOriginFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(origin)");
}

void BuiltinFlavorFunc(const vector<Value*>&, Evaluator*, string*) {
  printf("TODO(flavor)");
}

void BuiltinInfoFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  printf("%s\n", a->c_str());
  fflush(stdout);
}

void BuiltinWarningFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  printf("%s:%d: %s\n", LOCF(ev->loc()), a->c_str());
  fflush(stdout);
}

void BuiltinErrorFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  ev->Error(StringPrintf("*** %s.", a->c_str()));
}

FuncInfo g_func_infos[] = {
  { "patsubst", &BuiltinPatsubstFunc, 1 },
  { "strip", &BuiltinStripFunc, 1 },
  { "subst", &BuiltinSubstFunc, 1 },
  { "findstring", &BuiltinFindstringFunc, 1 },
  { "filter", &BuiltinFilterFunc, 1 },
  { "filter-out", &BuiltinFilterOutFunc, 1 },
  { "sort", &BuiltinSortFunc, 1 },
  { "word", &BuiltinWordFunc, 1 },
  { "wordlist", &BuiltinWordlistFunc, 1 },
  { "words", &BuiltinWordsFunc, 1 },
  { "firstword", &BuiltinFirstwordFunc, 1 },
  { "lastword", &BuiltinLastwordFunc, 1 },
  { "join", &BuiltinJoinFunc, 1 },
  { "wildcard", &BuiltinWildcardFunc, 1 },
  { "dir", &BuiltinDirFunc, 1 },
  { "notdir", &BuiltinNotdirFunc, 1 },
  { "suffix", &BuiltinSuffixFunc, 1 },
  { "basename", &BuiltinBasenameFunc, 1 },
  { "addsuffix", &BuiltinAddsuffixFunc, 1 },
  { "addprefix", &BuiltinAddprefixFunc, 1 },
  { "realpath", &BuiltinRealpathFunc, 1 },
  { "abspath", &BuiltinAbspathFunc, 1 },
  { "if", &BuiltinIfFunc, 1 },
  { "and", &BuiltinAndFunc, 1 },
  { "or", &BuiltinOrFunc, 1 },
  { "value", &BuiltinValueFunc, 1 },
  { "eval", &BuiltinEvalFunc, 1 },
  { "shell", &BuiltinShellFunc, 1 },
  { "call", &BuiltinCallFunc, 1 },
  { "foreach", &BuiltinForeachFunc, 1 },
  { "origin", &BuiltinOriginFunc, 1 },
  { "flavor", &BuiltinFlavorFunc, 1 },
  { "info", &BuiltinInfoFunc, 1 },
  { "warning", &BuiltinWarningFunc, 1 },
  { "error", &BuiltinErrorFunc, 1 },
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
