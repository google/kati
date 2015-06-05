#include "stringprintf.h"

#include <assert.h>
#include <stdarg.h>

string StringPrintf(const char* format, ...) {
  string str;
  str.resize(128);
  for (int i = 0; i < 2; i++) {
    va_list args;
    va_start(args, format);
    int ret = vsnprintf(&str[0], str.size(), format, args);
    va_end(args);
    assert(ret >= 0);
    if (static_cast<size_t>(ret) < str.size()) {
      str.resize(ret);
      return str;
    }
    str.resize(ret + 1);
  }
  assert(false);
}
