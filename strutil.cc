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

void AppendSubstPattern(StringPiece str, StringPiece pat, StringPiece subst,
                        string* out) {
  size_t pat_percent_index = pat.find('%');
  if (pat_percent_index == string::npos) {
    if (str == pat) {
      AppendString(subst, out);
      return;
    } else {
      AppendString(str, out);
      return;
    }
  }

  if (HasPrefix(str, pat.substr(0, pat_percent_index)) &&
      HasSuffix(str, pat.substr(pat_percent_index + 1))) {
    size_t subst_percent_index = subst.find('%');
    if (subst_percent_index == string::npos) {
      AppendString(subst, out);
      return;
    } else {
      AppendString(subst.substr(0, subst_percent_index), out);
      AppendString(str.substr(pat_percent_index,
                              str.size() - pat.size() + 1), out);
      AppendString(subst.substr(subst_percent_index + 1), out);
      return;
    }
  }
  AppendString(str, out);
}

void AppendSubstRef(StringPiece str, StringPiece pat, StringPiece subst,
                    string* out) {
  if (pat.find('%') != string::npos && subst.find('%') != string::npos) {
    AppendSubstPattern(str, pat, subst, out);
    return;
  }
  StringPiece s = TrimSuffix(str, pat);
  out->append(s.begin(), s.end());
  out->append(subst.begin(), subst.end());
}

string NoLineBreak(const string& s) {
  size_t index = s.find('\n');
  if (index == string::npos)
    return s;
  string r = s;
  while (index != string::npos) {
    r = s.substr(0, index) + "\\n" + s.substr(index + 1);
    index = s.find('\n', index + 2);
  }
  return r;
}
