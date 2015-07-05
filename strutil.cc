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

#include <stack>
#include <utility>

#include "log.h"

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
    if (isspace((*in)[i]))
      break;
  }
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
    if (isspace(s[i]))
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
    if (isspace(c)) {
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

void NormalizePath(string* o, size_t start_index) {
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
    } else if (prev_dir == "..") {
      j -= 4;
      j = o->rfind('/', j);
      if (j == string::npos) {
        j = start_index;
      } else {
        j++;
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
  NormalizePath(o, 1);
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
  bool prev_backslash = false;
  for (; e < s.size(); e++) {
    char c = s[e];
    if (c == '\\') {
      prev_backslash = !prev_backslash;
    } else if (c == '\n') {
      ++*lf_cnt;
      if (!prev_backslash) {
        return e;
      }
      prev_backslash = false;
    } else if (c != '\r') {
      prev_backslash = false;
    }
  }
  return e;
}

StringPiece TrimLeadingCurdir(StringPiece s) {
  while (s.substr(0, 2) == "./")
    s = s.substr(2);
  return s;
}
