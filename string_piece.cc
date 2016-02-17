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

// Copyright (c) 2011 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
// Copied from strings/stringpiece.cc with modifications

// +build ignore

#include <ctype.h>
#include <limits.h>
#include <stdint.h>

#include <algorithm>
#include <ostream>

#include "string_piece.h"

typedef StringPiece::size_type size_type;

bool operator==(const StringPiece& x, const StringPiece& y) {
  if (x.size() != y.size())
    return false;
  size_t len = x.size();
  if (len >= sizeof(uint64_t)) {
    len -= sizeof(uint64_t);
    uint64_t xt = *reinterpret_cast<const uint64_t*>(x.data() + len);
    uint64_t yt = *reinterpret_cast<const uint64_t*>(y.data() + len);
    if (xt != yt)
      return false;
  }
  return StringPiece::wordmemcmp(x.data(), y.data(), len) == 0;
}

void StringPiece::CopyToString(std::string* target) const {
  target->assign(!empty() ? data() : "", size());
}

void StringPiece::AppendToString(std::string* target) const {
  if (!empty())
    target->append(data(), size());
}

size_type StringPiece::copy(char* buf, size_type n, size_type pos) const {
  size_type ret = std::min(length_ - pos, n);
  memcpy(buf, ptr_ + pos, ret);
  return ret;
}

size_type StringPiece::find(const StringPiece& s, size_type pos) const {
  if (pos > length_)
    return npos;

  const char* result = std::search(ptr_ + pos, ptr_ + length_,
                                   s.ptr_, s.ptr_ + s.length_);
  const size_type xpos = result - ptr_;
  return xpos + s.length_ <= length_ ? xpos : npos;
}

size_type StringPiece::find(char c, size_type pos) const {
  if (pos >= length_)
    return npos;

  const char* result = std::find(ptr_ + pos, ptr_ + length_, c);
  return result != ptr_ + length_ ? static_cast<size_t>(result - ptr_) : npos;
}

size_type StringPiece::rfind(const StringPiece& s, size_type pos) const {
  if (length_ < s.length_)
    return npos;

  if (s.empty())
    return std::min(length_, pos);

  const char* last = ptr_ + std::min(length_ - s.length_, pos) + s.length_;
  const char* result = std::find_end(ptr_, last, s.ptr_, s.ptr_ + s.length_);
  return result != last ? static_cast<size_t>(result - ptr_) : npos;
}

size_type StringPiece::rfind(char c, size_type pos) const {
  if (length_ == 0)
    return npos;

  for (size_type i = std::min(pos, length_ - 1); ; --i) {
    if (ptr_[i] == c)
      return i;
    if (i == 0)
      break;
  }
  return npos;
}

// For each character in characters_wanted, sets the index corresponding
// to the ASCII code of that character to 1 in table.  This is used by
// the find_.*_of methods below to tell whether or not a character is in
// the lookup table in constant time.
// The argument `table' must be an array that is large enough to hold all
// the possible values of an unsigned char.  Thus it should be be declared
// as follows:
//   bool table[UCHAR_MAX + 1]
static inline void BuildLookupTable(const StringPiece& characters_wanted,
                                    bool* table) {
  const size_type length = characters_wanted.length();
  const char* const data = characters_wanted.data();
  for (size_type i = 0; i < length; ++i) {
    table[static_cast<unsigned char>(data[i])] = true;
  }
}

size_type StringPiece::find_first_of(const StringPiece& s,
                                     size_type pos) const {
  if (length_ == 0 || s.length_ == 0)
    return npos;

  // Avoid the cost of BuildLookupTable() for a single-character search.
  if (s.length_ == 1)
    return find_first_of(s.ptr_[0], pos);

  bool lookup[UCHAR_MAX + 1] = { false };
  BuildLookupTable(s, lookup);
  for (size_type i = pos; i < length_; ++i) {
    if (lookup[static_cast<unsigned char>(ptr_[i])]) {
      return i;
    }
  }
  return npos;
}

size_type StringPiece::find_first_not_of(const StringPiece& s,
                                         size_type pos) const {
  if (length_ == 0)
    return npos;

  if (s.length_ == 0)
    return 0;

  // Avoid the cost of BuildLookupTable() for a single-character search.
  if (s.length_ == 1)
    return find_first_not_of(s.ptr_[0], pos);

  bool lookup[UCHAR_MAX + 1] = { false };
  BuildLookupTable(s, lookup);
  for (size_type i = pos; i < length_; ++i) {
    if (!lookup[static_cast<unsigned char>(ptr_[i])]) {
      return i;
    }
  }
  return npos;
}

size_type StringPiece::find_first_not_of(char c, size_type pos) const {
  if (length_ == 0)
    return npos;

  for (; pos < length_; ++pos) {
    if (ptr_[pos] != c) {
      return pos;
    }
  }
  return npos;
}

size_type StringPiece::find_last_of(const StringPiece& s, size_type pos) const {
  if (length_ == 0 || s.length_ == 0)
    return npos;

  // Avoid the cost of BuildLookupTable() for a single-character search.
  if (s.length_ == 1)
    return find_last_of(s.ptr_[0], pos);

  bool lookup[UCHAR_MAX + 1] = { false };
  BuildLookupTable(s, lookup);
  for (size_type i = std::min(pos, length_ - 1); ; --i) {
    if (lookup[static_cast<unsigned char>(ptr_[i])])
      return i;
    if (i == 0)
      break;
  }
  return npos;
}

size_type StringPiece::find_last_not_of(const StringPiece& s,
                                        size_type pos) const {
  if (length_ == 0)
    return npos;

  size_type i = std::min(pos, length_ - 1);
  if (s.length_ == 0)
    return i;

  // Avoid the cost of BuildLookupTable() for a single-character search.
  if (s.length_ == 1)
    return find_last_not_of(s.ptr_[0], pos);

  bool lookup[UCHAR_MAX + 1] = { false };
  BuildLookupTable(s, lookup);
  for (; ; --i) {
    if (!lookup[static_cast<unsigned char>(ptr_[i])])
      return i;
    if (i == 0)
      break;
  }
  return npos;
}

size_type StringPiece::find_last_not_of(char c, size_type pos) const {
  if (length_ == 0)
    return npos;

  for (size_type i = std::min(pos, length_ - 1); ; --i) {
    if (ptr_[i] != c)
      return i;
    if (i == 0)
      break;
  }
  return npos;
}

StringPiece StringPiece::substr(size_type pos, size_type n) const {
  if (pos > length_) pos = length_;
  if (n > length_ - pos) n = length_ - pos;
  return StringPiece(ptr_ + pos, n);
}

const StringPiece::size_type StringPiece::npos = size_type(-1);
