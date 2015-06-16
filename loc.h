#ifndef LOC_H_
#define LOC_H_

#include <string>

#include "stringprintf.h"

struct Loc {
  Loc()
      : filename(0), lineno(-1) {}
  Loc(const char* f, int l)
      : filename(f), lineno(l) {
  }

  const char* filename;
  int lineno;
};

#define LOCF(x) (x).filename, (x).lineno

#endif  // LOC_H_
