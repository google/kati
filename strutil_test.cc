#include "strutil.h"

#include <assert.h>

#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

void TestWordScanner() {
  vector<StringPiece> ss;
  for (StringPiece tok : WordScanner("foo bar baz")) {
    ss.push_back(tok);
  }
  assert(ss.size() == 3LU);
  assert(ss[0] == "foo");
  assert(ss[1] == "bar");
  assert(ss[2] == "baz");
}

void TestHasPrefix() {
  assert(HasPrefix("foo", "foo"));
  assert(HasPrefix("foo", "fo"));
  assert(HasPrefix("foo", ""));
  assert(!HasPrefix("foo", "fooo"));
}

void TestHasSuffix() {
  assert(HasSuffix("bar", "bar"));
  assert(HasSuffix("bar", "ar"));
  assert(HasSuffix("bar", ""));
  assert(!HasSuffix("bar", "bbar"));
}

string SubstPattern(StringPiece str, StringPiece pat, StringPiece subst) {
  string r;
  AppendSubstPattern(str, pat, subst, &r);
  return r;
}

void TestSubstPattern() {
  assert(SubstPattern("x.c", "%.c", "%.o") == "x.o");
  assert(SubstPattern("c.x", "c.%", "o.%") == "o.x");
  assert(SubstPattern("x.c.c", "%.c", "%.o") == "x.c.o");
  assert(SubstPattern("x.x y.c", "%.c", "%.o") == "x.x y.o");
  assert(SubstPattern("x.%.c", "%.%.c", "OK") == "OK");
  assert(SubstPattern("x.c", "x.c", "OK") == "OK");
  assert(SubstPattern("x.c.c", "x.c", "XX") == "x.c.c");
  assert(SubstPattern("x.x.c", "x.c", "XX") == "x.x.c");
}

int main() {
  TestWordScanner();
  TestHasPrefix();
  TestHasSuffix();
  TestSubstPattern();
}
