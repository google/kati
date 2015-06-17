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

class WordWriter {
 public:
  explicit WordWriter(string* o);
  void MaybeAddWhitespace();
  void Write(StringPiece s);

 private:
  string* out_;
  bool needs_space_;
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

void AppendString(StringPiece str, string* out);

bool HasPrefix(StringPiece str, StringPiece prefix);

bool HasSuffix(StringPiece str, StringPiece suffix);

StringPiece TrimSuffix(StringPiece str, StringPiece suffix);

void AppendSubstPattern(StringPiece str, StringPiece pat, StringPiece subst,
                        string* out);

void AppendSubstRef(StringPiece str, StringPiece pat, StringPiece subst,
                    string* out);

string NoLineBreak(const string& s);

StringPiece TrimLeftSpace(StringPiece s);
StringPiece TrimRightSpace(StringPiece s);
StringPiece TrimSpace(StringPiece s);

#endif  // STRUTIL_H_
