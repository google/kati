#ifndef FUNC_H_
#define FUNC_H_

#include <vector>

#include "value.h"

using namespace std;

struct FuncInfo {
  const char* name;
  void (*func)(const vector<Value*>& args, Evaluator* ev, string* s);
  int arity;
};

void InitFuncTable();
void QuitFuncTable();

FuncInfo* GetFuncInfo(StringPiece name);

#endif  // FUNC_H_
