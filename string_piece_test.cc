#include "string_piece.h"

#include <assert.h>

#include <unordered_set>

using namespace std;

int main() {
  unordered_set<StringPiece> sps;
  sps.insert(STRING_PIECE("foo"));
  sps.insert(STRING_PIECE("foo"));
  sps.insert(STRING_PIECE("bar"));
  assert(sps.size() == 2);
  assert(sps.count(STRING_PIECE("foo")) == 1);
  assert(sps.count(STRING_PIECE("bar")) == 1);
}
