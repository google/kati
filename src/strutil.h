// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#ifndef STRUTIL_H_
#define STRUTIL_H_

#include <string>
#include <vector>

#include "string_piece.h"

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

  void Split(std::vector<StringPiece>* o);

 private:
  StringPiece in_;
};

class WordWriter {
 public:
  explicit WordWriter(std::string* o);
  void MaybeAddWhitespace();
  void Write(StringPiece s);

 private:
  std::string* out_;
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

template <class String>
inline std::string JoinStrings(std::vector<String> v, const char* sep) {
  std::string r;
  for (StringPiece s : v) {
    if (!r.empty()) {
      r += sep;
    }
    r.append(s.begin(), s.end());
  }
  return r;
}

void AppendString(StringPiece str, std::string* out);

bool HasPrefix(StringPiece str, StringPiece prefix);

bool HasSuffix(StringPiece str, StringPiece suffix);

bool HasWord(StringPiece str, StringPiece w);

StringPiece TrimPrefix(StringPiece str, StringPiece suffix);

StringPiece TrimSuffix(StringPiece str, StringPiece suffix);

class Pattern {
 public:
  explicit Pattern(StringPiece pat);

  bool Match(StringPiece str) const;

  StringPiece Stem(StringPiece str) const;

  void AppendSubst(StringPiece str, StringPiece subst, std::string* out) const;

  void AppendSubstRef(StringPiece str,
                      StringPiece subst,
                      std::string* out) const;

 private:
  bool MatchImpl(StringPiece str) const;

  StringPiece pat_;
  size_t percent_index_;
};

std::string NoLineBreak(const std::string& s);

StringPiece TrimLeftSpace(StringPiece s);
StringPiece TrimRightSpace(StringPiece s);
StringPiece TrimSpace(StringPiece s);

StringPiece Dirname(StringPiece s);
StringPiece Basename(StringPiece s);
StringPiece GetExt(StringPiece s);
StringPiece StripExt(StringPiece s);
void NormalizePath(std::string* o);
void AbsPath(StringPiece s, std::string* o);

size_t FindOutsideParen(StringPiece s, char c);
size_t FindTwoOutsideParen(StringPiece s, char c1, char c2);
size_t FindThreeOutsideParen(StringPiece s, char c1, char c2, char c3);

size_t FindEndOfLine(StringPiece s, size_t e, size_t* lf_cnt);

// Strip leading sequences of './' from file names, so that ./file
// and file are considered to be the same file.
// From http://www.gnu.org/software/make/manual/make.html#Features
StringPiece TrimLeadingCurdir(StringPiece s);

void FormatForCommandSubstitution(std::string* s);

std::string SortWordsInString(StringPiece s);

std::string ConcatDir(StringPiece b, StringPiece n);

std::string EchoEscape(const std::string& str);

void EscapeShell(std::string* s);

bool IsInteger(StringPiece s);

#endif  // STRUTIL_H_
