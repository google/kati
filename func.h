#ifndef FUNC_H_
#define FUNC_H_

#include <vector>

#include "value.h"

using namespace std;

struct FuncInfo {
  const char* name;
  void (*func)(const vector<Value*>& args, Evaluator* ev, string* s);
  int arity;
  int min_arity;
  // For all parameters.
  bool trim_space;
  // Only for the first parameter.
  bool trim_right_space_1st;
};

void InitFuncTable();
void QuitFuncTable();

FuncInfo* GetFuncInfo(StringPiece name);

#endif  // FUNC_H_
