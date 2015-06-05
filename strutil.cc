#include "strutil.h"

#include <ctype.h>
#include <string.h>

#include <unordered_map>
#include <utility>

WordScanner::Iterator& WordScanner::Iterator::operator++() {
  int len = static_cast<int>(in->size());
  for (s = i; s < len; s++) {
    if (!isspace((*in)[s]))
      break;
  }
  if (s == len) {
    in = NULL;
    s = 0;
    i = 0;
    return *this;
  }
  for (i = s; i < len; i++) {
    if (isspace((*in)[s]))
      break;
  }
  return *this;
}

StringPiece WordScanner::Iterator::operator*() const {
  return in->substr(s, i);
}

WordScanner::WordScanner(StringPiece in)
    : in_(in) {
}

WordScanner::Iterator WordScanner::begin() const {
  Iterator iter;
  iter.in = &in_;
  iter.s = 0;
  iter.i = 0;
  ++iter;
  return iter;
}

WordScanner::Iterator WordScanner::end() const {
  Iterator iter;
  iter.in = NULL;
  iter.s = 0;
  iter.i = 0;
  return iter;
}

static unordered_map<StringPiece, char*>* g_symtab;

void InitSymtab() {
  g_symtab = new unordered_map<StringPiece, char*>;
}

void QuitSymtab() {
  for (auto p : *g_symtab) {
    free(p.second);
  }
  delete g_symtab;
}

StringPiece Intern(StringPiece s) {
  auto found = g_symtab->find(s);
  if (found != g_symtab->end())
    return found->first;

  char* b = static_cast<char*>(malloc(s.size()+1));
  memcpy(b, s.data(), s.size());
  s = StringPiece(b, s.size());
  (*g_symtab)[s] = b;
  return s;
}
