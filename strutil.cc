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

// +build ignore

#include "strutil.h"

#include <ctype.h>
#include <limits.h>
#include <unistd.h>

#include <algorithm>
#include <functional>
#include <stack>
#include <utility>

#ifdef __SSE4_2__
#include <smmintrin.h>
#endif

#include "log.h"

static bool isSpace(char c) {
  return (9 <= c && c <= 13) || c == 32;
}

#ifdef __SSE4_2__
static int SkipUntilSSE42(const char* s, int len,
                          const char* ranges, int ranges_size) {
  __m128i ranges16 = _mm_loadu_si128((const __m128i*)ranges);
  len &= ~15;
  int i = 0;
  while (i < len) {
    __m128i b16 = _mm_loadu_si128((const __m128i*)(s + i));
    int r = _mm_cmpestri(
        ranges16, ranges_size, b16, len - i,
        _SIDD_LEAST_SIGNIFICANT | _SIDD_CMP_RANGES | _SIDD_UBYTE_OPS);
    if (r != 16) {
      return i + r;
    }
    i += 16;
  }
  return len;
}
#endif

template <typename Cond>
static int SkipUntil(const char* s, int len,
                     const char* ranges, int ranges_size,
                     Cond cond) {
  int i = 0;
#ifdef __SSE4_2__
  i += SkipUntilSSE42(s, len, ranges, ranges_size);
#endif
  for (; i < len; i++) {
    if (cond(s[i]))
      break;
  }
  return i;
}

WordScanner::Iterator& WordScanner::Iterator::operator++() {
  int len = static_cast<int>(in->size());
  for (s = i + 1; s < len; s++) {
    if (!isSpace((*in)[s]))
      break;
  }
  if (s >= len) {
    in = NULL;
    s = 0;
    i = 0;
    return *this;
  }

  static const char ranges[] = "\x09\x0d  ";
  // It's intentional we are not using isSpace here. It seems with
  // lambda the compiler generates better code.
  i = s + SkipUntil(in->data() + s, len - s, ranges, 4,
                    [](char c) { return (9 <= c && c <= 13) || c == 32; });
  return *this;
}

StringPiece WordScanner::Iterator::operator*() const {
  return in->substr(s, i - s);
}

WordScanner::WordScanner(StringPiece in)
    : in_(in) {
}

WordScanner::Iterator WordScanner::begin() const {
  Iterator iter;
  iter.in = &in_;
  iter.s = 0;
  iter.i = -1;
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

void WordScanner::Split(vector<StringPiece>* o) {
  for (StringPiece t : *this)
    o->push_back(t);
}

WordWriter::WordWriter(string* o)
    : out_(o),
      needs_space_(false) {
}

void WordWriter::MaybeAddWhitespace() {
  if (needs_space_) {
    out_->push_back(' ');
  } else {
    needs_space_ = true;
  }
}

void WordWriter::Write(StringPiece s) {
  MaybeAddWhitespace();
  AppendString(s, out_);
}

ScopedTerminator::ScopedTerminator(StringPiece s)
    : s_(s), c_(s[s.size()]) {
  const_cast<char*>(s_.data())[s_.size()] = '\0';
}

ScopedTerminator::~ScopedTerminator() {
  const_cast<char*>(s_.data())[s_.size()] = c_;
}

void AppendString(StringPiece str, string* out) {
  out->append(str.begin(), str.end());
}

bool HasPrefix(StringPiece str, StringPiece prefix) {
  ssize_t size_diff = str.size() - prefix.size();
  return size_diff >= 0 && str.substr(0, prefix.size()) == prefix;
}

bool HasSuffix(StringPiece str, StringPiece suffix) {
  ssize_t size_diff = str.size() - suffix.size();
  return size_diff >= 0 && str.substr(size_diff) == suffix;
}

bool HasWord(StringPiece str, StringPiece w) {
  size_t found = str.find(w);
  if (found == string::npos)
    return false;
  if (found != 0 && !isSpace(str[found-1]))
    return false;
  size_t end = found + w.size();
  if (end != str.size() && !isSpace(str[end]))
    return false;
  return true;
}

StringPiece TrimSuffix(StringPiece str, StringPiece suffix) {
  ssize_t size_diff = str.size() - suffix.size();
  if (size_diff < 0 || str.substr(size_diff) != suffix)
    return str;
  return str.substr(0, size_diff);
}

Pattern::Pattern(StringPiece pat)
    : pat_(pat), percent_index_(pat.find('%')) {
}

bool Pattern::Match(StringPiece str) const {
  if (percent_index_ == string::npos)
    return str == pat_;
  return MatchImpl(str);
}

bool Pattern::MatchImpl(StringPiece str) const {
  return (HasPrefix(str, pat_.substr(0, percent_index_)) &&
          HasSuffix(str, pat_.substr(percent_index_ + 1)));
}

StringPiece Pattern::Stem(StringPiece str) const {
  if (!Match(str))
    return "";
  return str.substr(percent_index_,
                    str.size() - (pat_.size() - percent_index_ - 1));
}

void Pattern::AppendSubst(StringPiece str, StringPiece subst,
                          string* out) const {
  if (percent_index_ == string::npos) {
    if (str == pat_) {
      AppendString(subst, out);
      return;
    } else {
      AppendString(str, out);
      return;
    }
  }

  if (MatchImpl(str)) {
    size_t subst_percent_index = subst.find('%');
    if (subst_percent_index == string::npos) {
      AppendString(subst, out);
      return;
    } else {
      AppendString(subst.substr(0, subst_percent_index), out);
      AppendString(str.substr(percent_index_,
                              str.size() - pat_.size() + 1), out);
      AppendString(subst.substr(subst_percent_index + 1), out);
      return;
    }
  }
  AppendString(str, out);
}

void Pattern::AppendSubstRef(StringPiece str, StringPiece subst,
                             string* out) const {
  if (percent_index_ != string::npos && subst.find('%') != string::npos) {
    AppendSubst(str, subst, out);
    return;
  }
  StringPiece s = TrimSuffix(str, pat_);
  out->append(s.begin(), s.end());
  out->append(subst.begin(), subst.end());
}

string NoLineBreak(const string& s) {
  size_t index = s.find('\n');
  if (index == string::npos)
    return s;
  string r = s;
  while (index != string::npos) {
    r = r.substr(0, index) + "\\n" + r.substr(index + 1);
    index = r.find('\n', index + 2);
  }
  return r;
}

StringPiece TrimLeftSpace(StringPiece s) {
  size_t i = 0;
  for (; i < s.size(); i++) {
    if (isSpace(s[i]))
      continue;
    char n = s.get(i+1);
    if (s[i] == '\\' && (n == '\r' || n == '\n')) {
      i++;
      continue;
    }
    break;
  }
  return s.substr(i, s.size() - i);
}

StringPiece TrimRightSpace(StringPiece s) {
  size_t i = 0;
  for (; i < s.size(); i++) {
    char c = s[s.size() - 1 - i];
    if (isSpace(c)) {
      if ((c == '\r' || c == '\n') && s.get(s.size() - 2 - i) == '\\')
        i++;
      continue;
    }
    break;
  }
  return s.substr(0, s.size() - i);
}

StringPiece TrimSpace(StringPiece s) {
  return TrimRightSpace(TrimLeftSpace(s));
}

StringPiece Dirname(StringPiece s) {
  size_t found = s.rfind('/');
  if (found == string::npos)
    return StringPiece(".");
  if (found == 0)
    return StringPiece("");
  return s.substr(0, found);
}

StringPiece Basename(StringPiece s) {
  size_t found = s.rfind('/');
  if (found == string::npos || found == 0)
    return s;
  return s.substr(found + 1);
}

StringPiece GetExt(StringPiece s) {
  size_t found = s.rfind('.');
  if (found == string::npos)
    return StringPiece("");
  return s.substr(found);
}

StringPiece StripExt(StringPiece s) {
  size_t slash_index = s.rfind('/');
  size_t found = s.rfind('.');
  if (found == string::npos ||
      (slash_index != string::npos && found < slash_index))
    return s;
  return s.substr(0, found);
}

void NormalizePath(string* o) {
  if (o->empty())
    return;
  size_t start_index = 0;
  if ((*o)[0] == '/')
    start_index++;
  size_t j = start_index;
  size_t prev_start = start_index;
  for (size_t i = start_index; i <= o->size(); i++) {
    char c = (*o)[i];
    if (c != '/' && c != 0) {
      (*o)[j] = c;
      j++;
      continue;
    }

    StringPiece prev_dir = StringPiece(o->data() + prev_start, j - prev_start);
    if (prev_dir == ".") {
      j--;
    } else if (prev_dir == ".." && j != 2 /* .. */) {
      if (j == 3) {
        // /..
        j = start_index;
      } else {
        size_t orig_j = j;
        j -= 4;
        j = o->rfind('/', j);
        if (j == string::npos) {
          j = start_index;
        } else {
          j++;
        }
        if (StringPiece(o->data() + j, 3) == "../") {
          j = orig_j;
          (*o)[j] = c;
          j++;
        }
      }
    } else if (!prev_dir.empty()) {
      if (c) {
        (*o)[j] = c;
        j++;
      }
    }
    prev_start = j;
  }
  if (j > 1 && (*o)[j-1] == '/')
    j--;
  o->resize(j);
}

void AbsPath(StringPiece s, string* o) {
  if (s.get(0) == '/') {
    o->clear();
  } else {
    char buf[PATH_MAX];
    if (!getcwd(buf, PATH_MAX)) {
      fprintf(stderr, "getcwd failed\n");
      CHECK(false);
    }

    CHECK(buf[0] == '/');
    *o = buf;
    *o += '/';
  }
  AppendString(s, o);
  NormalizePath(o);
}

template<typename Cond>
size_t FindOutsideParenImpl(StringPiece s, Cond cond) {
  bool prev_backslash = false;
  stack<char> paren_stack;
  for (size_t i = 0; i < s.size(); i++) {
    char c = s[i];
    if (cond(c) && paren_stack.empty() && !prev_backslash) {
      return i;
    }
    switch (c) {
      case '(':
        paren_stack.push(')');
        break;
      case '{':
        paren_stack.push('}');
        break;

      case ')':
      case '}':
        if (!paren_stack.empty() && c == paren_stack.top()) {
          paren_stack.pop();
        }
        break;
    }
    prev_backslash = c == '\\' && !prev_backslash;
  }
  return string::npos;
}

size_t FindOutsideParen(StringPiece s, char c) {
  return FindOutsideParenImpl(s, [&c](char d){return c == d;});
}

size_t FindTwoOutsideParen(StringPiece s, char c1, char c2) {
  return FindOutsideParenImpl(s, [&c1, &c2](char d){
      return d == c1 || d == c2;
    });
}

size_t FindThreeOutsideParen(StringPiece s, char c1, char c2, char c3) {
  return FindOutsideParenImpl(s, [&c1, &c2, &c3](char d){
      return d == c1 || d == c2 || d == c3;
    });
}

size_t FindEndOfLine(StringPiece s, size_t e, size_t* lf_cnt) {
  static const char ranges[] = "\0\0\n\n\\\\";
  while (e < s.size()) {
    e += SkipUntil(s.data() + e, s.size() - e, ranges, 6,
                   [](char c) { return c == 0 || c == '\n' || c == '\\'; });
    if (e >= s.size()) {
      CHECK(s.size() == e);
      break;
    }
    char c = s[e];
    if (c == '\0')
      break;
    if (c == '\\') {
      if (s[e+1] == '\n') {
        e += 2;
        ++*lf_cnt;
      } else if (s[e+1] == '\r' && s[e+2] == '\n') {
        e += 3;
        ++*lf_cnt;
      } else if (s[e+1] == '\\') {
        e += 2;
      } else {
        e++;
      }
    } else if (c == '\n') {
      ++*lf_cnt;
      return e;
    }
  }
  return e;
}

StringPiece TrimLeadingCurdir(StringPiece s) {
  while (s.substr(0, 2) == "./")
    s = s.substr(2);
  return s;
}

void FormatForCommandSubstitution(string* s) {
  while ((*s)[s->size()-1] == '\n')
    s->pop_back();
  for (size_t i = 0; i < s->size(); i++) {
    if ((*s)[i] == '\n')
      (*s)[i] = ' ';
  }
}

string SortWordsInString(StringPiece s) {
  vector<string> toks;
  for (StringPiece tok : WordScanner(s)) {
    toks.push_back(tok.as_string());
  }
  sort(toks.begin(), toks.end());
  return JoinStrings(toks, " ");
}

string ConcatDir(StringPiece b, StringPiece n) {
  string r;
  if (!b.empty()) {
    b.AppendToString(&r);
    r += '/';
  }
  n.AppendToString(&r);
  NormalizePath(&r);
  return r;
}

string EchoEscape(const string str) {
  const char *in = str.c_str();
  string buf;
  for (; *in; in++) {
    switch(*in) {
      case '\\':
        buf += "\\\\\\\\";
        break;
      case '\n':
        buf += "\\n";
        break;
      case '"':
        buf += "\\\"";
        break;
      default:
        buf += *in;
    }
  }
  return buf;
}

static bool NeedsShellEscape(char c) {
  return c == 0 || c == '"' || c == '$' || c == '\\' || c == '`';
}

void EscapeShell(string* s) {
  static const char ranges[] = "\0\0\"\"$$\\\\``";
  size_t prev = 0;
  size_t i = SkipUntil(s->c_str(), s->size(), ranges, 10, NeedsShellEscape);
  if (i == s->size())
    return;

  string r;
  for (; i < s->size();) {
    StringPiece(*s).substr(prev, i - prev).AppendToString(&r);
    char c = (*s)[i];
    r += '\\';
    if (c == '$') {
      if ((*s)[i+1] == '$') {
        r += '$';
        i++;
      }
    }
    r += c;
    i++;
    prev = i;
    i += SkipUntil(s->c_str() + i, s->size() - i, ranges, 10, NeedsShellEscape);
  }
  StringPiece(*s).substr(prev).AppendToString(&r);
  s->swap(r);
}
