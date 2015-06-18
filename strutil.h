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

  void Split(vector<StringPiece>* o);

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

// Temporary modifies s[s.size()] to '\0'.
class ScopedTerminator {
 public:
  explicit ScopedTerminator(StringPiece s);
  ~ScopedTerminator();

 private:
  StringPiece s_;
  char c_;
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

bool MatchPattern(StringPiece str, StringPiece pat);

string NoLineBreak(const string& s);

StringPiece TrimLeftSpace(StringPiece s);
StringPiece TrimRightSpace(StringPiece s);
StringPiece TrimSpace(StringPiece s);

StringPiece Dirname(StringPiece s);
StringPiece Basename(StringPiece s);
StringPiece GetExt(StringPiece s);
StringPiece StripExt(StringPiece s);
void AbsPath(StringPiece s, string* o);

size_t FindOutsideParen(StringPiece s, char c);
size_t FindTwoOutsideParen(StringPiece s, char c1, char c2);

#endif  // STRUTIL_H_
