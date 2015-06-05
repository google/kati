#ifndef STRUTIL_H_
#define STRUTIL_H_

#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

class WordScanner {
 public:
  struct Iterator {
    Iterator& operator++();
    StringPiece operator*() const;
    bool operator!=(const Iterator& r) const {
      return in != r.in || s != r.s || i != r.i;
    }

    const StringPiece* in;
    int s;
    int i;
  };

  explicit WordScanner(StringPiece in);

  Iterator begin() const;
  Iterator end() const;

 private:
  StringPiece in_;
};

void InitSymtab();
void QuitSymtab();
StringPiece Intern(StringPiece s);

template <class String>
inline string JoinStrings(vector<String> v, const char* sep) {
  string r;
  for (StringPiece s : v) {
    if (!r.empty()) {
      r += sep;
    }
    r.append(s.begin(), s.end());
  }
  return r;
}

#endif  // STRUTIL_H_
