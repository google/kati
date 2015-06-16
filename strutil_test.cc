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

int main() {
  TestWordScanner();
}
