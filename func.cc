#include "func.h"

#include <stdio.h>

#include <unordered_map>

#include "eval.h"
#include "log.h"
#include "strutil.h"

namespace {

void BuiltinInfoFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  printf("%s\n", a->c_str());
  fflush(stdout);
}

void BuiltinWarningFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  printf("%s:%d: %s\n", ev->loc().filename, ev->loc().lineno, a->c_str());
  fflush(stdout);
}

void BuiltinErrorFunc(const vector<Value*>& args, Evaluator* ev, string*) {
  shared_ptr<string> a = args[0]->Eval(ev);
  ev->Error(StringPrintf("*** %s.", a->c_str()));
}

FuncInfo g_func_infos[] = {
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
