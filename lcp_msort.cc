// Copyright 2016 Google Inc. All rights reserved
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

// http://ci.nii.ac.jp/naid/110006613087

#include "lcp_msort.h"

#include <stdlib.h>

#include "strutil.h"

#define DO_SORT_AND_UNIQ_AT_ONCE

namespace {

struct AnnotatedString {
  const unsigned char* s;
  int l;
};

inline int LcpCompare(const unsigned char* s1,
                      const unsigned char* s2,
                      int b,
                      int& i) {
  for (i = b; s1[i] || s2[i]; i++) {
    unsigned char c1 = s1[i];
    unsigned char c2 = s2[i];
    if (c1 < c2) {
      return -1;
    }
    else if (c1 > c2) {
      return 1;
    }
  }
  return 0;
}

#ifdef DO_SORT_AND_UNIQ_AT_ONCE

int StringMergeSortAndUniq(const vector<const unsigned char*>& data,
                           int begin,
                           int end,
                           AnnotatedString* work,
                           AnnotatedString* out) {
  if (begin + 1 == end) {
    out[begin] = AnnotatedString{ data[begin], 0 };
    return 1;
  }

  int mid = (begin + end) / 2;
  int s1_len = StringMergeSortAndUniq(data, begin, mid, work, out);
  int s2_len = StringMergeSortAndUniq(data, mid, end, work, out);

  memcpy(work, out + begin, s1_len * sizeof(AnnotatedString));

  AnnotatedString* s1 = work;
  AnnotatedString* s2 = out + mid;
  int i = 0, j = 0, k = 0;
  AnnotatedString* d = out + begin;
  for (; i < s1_len && j < s2_len; k++) {
    if (s1[i].l > s2[j].l) {
      d[k] = s1[i];
      i++;
    } else if (s1[i].l < s2[j].l) {
      d[k] = s2[j];
      j++;
    } else {
      int m;
      int r = LcpCompare(s1[i].s, s2[j].s, s1[i].l, m);
      if (r < 0) {
        d[k] = s1[i];
        i++;
        s2[j].l = m;
      } else if (r > 0) {
        d[k] = s2[j];
        j++;
        s1[i].l = m;
      } else {
        // Discard a unique string.
        d[k] = s1[i];
        i++;
        j++;
      }
    }
  }

  if (i < s1_len) {
    memcpy(&d[k], &s1[i], (s1_len - i) * sizeof(AnnotatedString));
    k += s1_len - i;
  }
  else if (j < s2_len) {
    memcpy(&d[k], &s2[j], (s2_len - j) * sizeof(AnnotatedString));
    k += s2_len - j;
  }
  return k;
}

#else

void StringMergeSort(const vector<const unsigned char*>& data,
                     int begin,
                     int end,
                     AnnotatedString* work,
                     AnnotatedString* out) {
  if (begin + 1 == end) {
    out[begin] = AnnotatedString{ data[begin], 0 };
    return;
  }

  int mid = (begin + end) / 2;
  StringMergeSort(data, begin, mid, work, out);
  StringMergeSort(data, mid, end, work, out);

  memcpy(work, out + begin, (mid - begin) * sizeof(AnnotatedString));

  AnnotatedString* s1 = work;
  int s1_len = mid - begin;
  AnnotatedString* s2 = out + mid;
  int s2_len = end - mid;
  int i = 0, j = 0, k = 0;
  AnnotatedString* d = out + begin;
  for (; i < s1_len && j < s2_len; k++) {
    if (s1[i].l > s2[j].l) {
      d[k] = s1[i];
      i++;
    } else if (s1[i].l < s2[j].l) {
      d[k] = s2[j];
      j++;
    } else {
      int m;
      int r = LcpCompare(s1[i].s, s2[j].s, s1[i].l, m);
      if (r < 0) {
        d[k] = s1[i];
        i++;
        s2[j].l = m;
      } else {
        d[k] = s2[j];
        j++;
        s1[i].l = m;
      }
    }
  }

  if (i < s1_len)
    memcpy(&d[k], &s1[i], (s1_len - i) * sizeof(AnnotatedString));
  else if (j < s2_len)
    memcpy(&d[k], &s2[j], (s2_len - j) * sizeof(AnnotatedString));
}

#endif

}  // namespace

void StringSortByLcpMsort(string* buf, string* out) {
  vector<const unsigned char*> toks;
  for (StringPiece tok : WordScanner(*buf)) {
    const_cast<char*>(tok.data())[tok.size()] = 0;
    toks.push_back(reinterpret_cast<const unsigned char*>(tok.data()));
  }

  if (toks.empty())
    return;
  if (toks.size() == 1) {
    *out += reinterpret_cast<const char*>(toks[0]);
    return;
  }

  AnnotatedString* as = static_cast<AnnotatedString*>(
      malloc(toks.size() * sizeof(AnnotatedString)));
  AnnotatedString* work = static_cast<AnnotatedString*>(
      malloc((toks.size() / 2 + 1) * sizeof(AnnotatedString)));

#ifdef DO_SORT_AND_UNIQ_AT_ONCE
  int len = StringMergeSortAndUniq(toks, 0, toks.size(), work, as);
#else
  int len = static_cast<int>(toks.size());
  StringMergeSort(toks, 0, toks.size(), work, as);
#endif

  WordWriter ww(out);
#ifdef DO_SORT_AND_UNIQ_AT_ONCE
  for (int i = 0; i < len; i++) {
    ww.MaybeAddWhitespace();
    *out += reinterpret_cast<const char*>(as[i].s);
  }
#else
  StringPiece prev;
  for (int i = 0; i < len; i++) {
    StringPiece tok = reinterpret_cast<const char*>(as[i].s);
    if (prev != tok) {
      ww.Write(tok);
      prev = tok;
    }
  }
#endif

  free(as);
  free(work);
}
