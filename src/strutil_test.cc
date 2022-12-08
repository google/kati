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

#include <assert.h>
#include <sys/mman.h>
#include <unistd.h>

#include <string>
#include <string_view>
#include <vector>

#include "testutil.h"

namespace {

void TestWordScanner() {
  std::vector<std::string_view> ss;
  for (std::string_view tok : WordScanner("foo bar baz hogeeeeeeeeeeeeeeee")) {
    ss.push_back(tok);
  }
  assert(ss.size() == 4LU);
  ASSERT_EQ(ss[0], "foo");
  ASSERT_EQ(ss[1], "bar");
  ASSERT_EQ(ss[2], "baz");
  ASSERT_EQ(ss[3], "hogeeeeeeeeeeeeeeee");
}

void TestHasPrefix() {
  assert(HasPrefix("foo", "foo"));
  assert(HasPrefix("foo", "fo"));
  assert(HasPrefix("foo", ""));
  assert(!HasPrefix("foo", "fooo"));
}

void TestHasSuffix() {
  assert(HasSuffix("bar", "bar"));
  assert(HasSuffix("bar", "ar"));
  assert(HasSuffix("bar", ""));
  assert(!HasSuffix("bar", "bbar"));
}

void TestTrimPrefix() {
  ASSERT_EQ(TrimPrefix("foo", "foo"), "");
  ASSERT_EQ(TrimPrefix("foo", "fo"), "o");
  ASSERT_EQ(TrimPrefix("foo", ""), "foo");
  ASSERT_EQ(TrimPrefix("foo", "fooo"), "foo");
}

void TestTrimSuffix() {
  ASSERT_EQ(TrimSuffix("bar", "bar"), "");
  ASSERT_EQ(TrimSuffix("bar", "ar"), "b");
  ASSERT_EQ(TrimSuffix("bar", ""), "bar");
  ASSERT_EQ(TrimSuffix("bar", "bbar"), "bar");
}

std::string SubstPattern(std::string_view str,
                         std::string_view pat,
                         std::string_view subst) {
  std::string r;
  Pattern(pat).AppendSubst(str, subst, &r);
  return r;
}

void TestSubstPattern() {
  ASSERT_EQ(SubstPattern("x.c", "%.c", "%.o"), "x.o");
  ASSERT_EQ(SubstPattern("c.x", "c.%", "o.%"), "o.x");
  ASSERT_EQ(SubstPattern("x.c.c", "%.c", "%.o"), "x.c.o");
  ASSERT_EQ(SubstPattern("x.x y.c", "%.c", "%.o"), "x.x y.o");
  ASSERT_EQ(SubstPattern("x.%.c", "%.%.c", "OK"), "OK");
  ASSERT_EQ(SubstPattern("x.c", "x.c", "OK"), "OK");
  ASSERT_EQ(SubstPattern("x.c.c", "x.c", "XX"), "x.c.c");
  ASSERT_EQ(SubstPattern("x.x.c", "x.c", "XX"), "x.x.c");
}

void TestNoLineBreak() {
  assert(NoLineBreak("a\nb") == "a\\nb");
  assert(NoLineBreak("a\nb\nc") == "a\\nb\\nc");
}

void TestHasWord() {
  assert(HasWord("foo bar baz", "bar"));
  assert(HasWord("foo bar baz", "foo"));
  assert(HasWord("foo bar baz", "baz"));
  assert(!HasWord("foo bar baz", "oo"));
  assert(!HasWord("foo bar baz", "ar"));
  assert(!HasWord("foo bar baz", "ba"));
  assert(!HasWord("foo bar baz", "az"));
  assert(!HasWord("foo bar baz", "ba"));
  assert(!HasWord("foo bar baz", "fo"));
}

static std::string NormalizePath(std::string s) {
  ::NormalizePath(&s);
  return s;
}

void TestNormalizePath() {
  ASSERT_EQ(NormalizePath(""), "");
  ASSERT_EQ(NormalizePath("."), "");
  ASSERT_EQ(NormalizePath("/"), "/");
  ASSERT_EQ(NormalizePath("/tmp"), "/tmp");
  ASSERT_EQ(NormalizePath("////tmp////"), "/tmp");
  ASSERT_EQ(NormalizePath("a////b"), "a/b");
  ASSERT_EQ(NormalizePath("a//.//b"), "a/b");
  ASSERT_EQ(NormalizePath("a////b//../c/////"), "a/c");
  ASSERT_EQ(NormalizePath("../foo"), "../foo");
  ASSERT_EQ(NormalizePath("./foo"), "foo");
  ASSERT_EQ(NormalizePath("x/y/..//../foo"), "foo");
  ASSERT_EQ(NormalizePath("x/../../foo"), "../foo");
  ASSERT_EQ(NormalizePath("/../foo"), "/foo");
  ASSERT_EQ(NormalizePath("/../../foo"), "/foo");
  ASSERT_EQ(NormalizePath("/a/../../foo"), "/foo");
  ASSERT_EQ(NormalizePath("/a/b/.."), "/a");
  ASSERT_EQ(NormalizePath("../../a/b"), "../../a/b");
  ASSERT_EQ(NormalizePath("../../../a/b"), "../../../a/b");
  ASSERT_EQ(NormalizePath(".././../a/b"), "../../a/b");
  ASSERT_EQ(NormalizePath("./../../a/b"), "../../a/b");
}

std::string EscapeShell(std::string s) {
  ::EscapeShell(&s);
  return s;
}

void TestEscapeShell() {
  ASSERT_EQ(EscapeShell(""), "");
  ASSERT_EQ(EscapeShell("foo"), "foo");
  ASSERT_EQ(EscapeShell("foo$`\\baz\"bar"), "foo\\$\\`\\\\baz\\\"bar");
  ASSERT_EQ(EscapeShell("$$"), "\\$$");
  ASSERT_EQ(EscapeShell("$$$"), "\\$$\\$");
  ASSERT_EQ(EscapeShell("\\\n"), "\\\\\n");
}

void TestFindEndOfLine() {
  size_t lf_cnt = 0;
  ASSERT_EQ(FindEndOfLine("foo", 0, &lf_cnt), 3);
  char buf[10] = {'f', 'o', '\\', '\0', 'x', 'y'};
  ASSERT_EQ(FindEndOfLine(std::string_view(buf, 6), 0, &lf_cnt), 3);
  ASSERT_EQ(FindEndOfLine(std::string_view(buf, 2), 0, &lf_cnt), 2);
}

// Take a string, and copy it into an allocated buffer where
// the byte immediately after the null termination character
// is read protected. Useful for testing, but doesn't support
// freeing the allocated pages.
const char* CreateProtectedString(const char* str) {
  int pagesize = sysconf(_SC_PAGE_SIZE);
  void* buffer;
  char* buffer_str;

  // Allocate two pages of memory
  if (posix_memalign(&buffer, pagesize, pagesize * 2) != 0) {
    perror("posix_memalign failed");
    assert(false);
  }

  // Make the second page unreadable
  buffer_str = (char*)buffer + pagesize;
  if (mprotect(buffer_str, pagesize, PROT_NONE) != 0) {
    perror("mprotect failed");
    assert(false);
  }

  // Then move the test string into the very end of the first page
  buffer_str -= strlen(str) + 1;
  strcpy(buffer_str, str);

  return buffer_str;
}

void TestWordScannerInvalidAccess() {
  std::vector<std::string_view> ss;
  for (std::string_view tok :
       WordScanner(CreateProtectedString("0123 456789"))) {
    ss.push_back(tok);
  }
  assert(ss.size() == 2LU);
  ASSERT_EQ(ss[0], "0123");
  ASSERT_EQ(ss[1], "456789");
}

void TestFindEndOfLineInvalidAccess() {
  size_t lf_cnt = 0;
  ASSERT_EQ(FindEndOfLine(CreateProtectedString("a\\"), 0, &lf_cnt), 2);
}

void TestConcatDir() {
  ASSERT_EQ(ConcatDir("", ""), "");
  ASSERT_EQ(ConcatDir(".", ""), "");
  ASSERT_EQ(ConcatDir("", "."), "");
  ASSERT_EQ(ConcatDir("a", "b"), "a/b");
  ASSERT_EQ(ConcatDir("a/", "b"), "a/b");
  ASSERT_EQ(ConcatDir("a", "/b"), "/b");
  ASSERT_EQ(ConcatDir("a", ".."), "");
  ASSERT_EQ(ConcatDir("a", "../b"), "b");
  ASSERT_EQ(ConcatDir("a", "../../b"), "../b");
}

void TestIsInteger() {
  ASSERT_BOOL(IsInteger("0"), true);
  ASSERT_BOOL(IsInteger("9"), true);
  ASSERT_BOOL(IsInteger("1234"), true);
  ASSERT_BOOL(IsInteger(""), false);
  ASSERT_BOOL(IsInteger("a234"), false);
  ASSERT_BOOL(IsInteger("123a"), false);
  ASSERT_BOOL(IsInteger("12a4"), false);
}

}  // namespace

int main() {
  TestWordScanner();
  TestHasPrefix();
  TestHasSuffix();
  TestTrimPrefix();
  TestTrimSuffix();
  TestSubstPattern();
  TestNoLineBreak();
  TestHasWord();
  TestNormalizePath();
  TestEscapeShell();
  TestFindEndOfLine();
  TestWordScannerInvalidAccess();
  TestFindEndOfLineInvalidAccess();
  TestConcatDir();
  TestIsInteger();
  assert(!g_failed);
}
