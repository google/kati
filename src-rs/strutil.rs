/*
Copyright 2025 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

use anyhow::Result;
use bytes::{BufMut, Bytes, BytesMut};
use memchr::{memchr, memchr2, memmem, memrchr};
use std::{borrow::Cow, env::current_dir, os::unix::ffi::OsStrExt};

pub fn is_space(c: char) -> bool {
    ('\t'..='\r').contains(&c) || c == ' '
}

pub fn is_space_byte(c: &u8) -> bool {
    let c = *c;
    (b'\t'..=b'\r').contains(&c) || c == b' '
}

pub fn skip_until(s: &[u8], pattern: &[u8]) -> usize {
    s.iter()
        .position(|c| pattern.contains(c))
        .unwrap_or(s.len())
}

pub fn skip_until2(s: &[u8], needle1: u8, needle2: u8) -> usize {
    memchr2(needle1, needle2, s).unwrap_or(s.len())
}

pub fn word_scanner(s: &[u8]) -> impl Iterator<Item = &[u8]> {
    s.split(is_space_byte).filter(|s| !s.is_empty())
}

pub struct WordWriter<'a> {
    pub out: &'a mut dyn BufMut,
    needs_space: bool,
}

impl<'a> WordWriter<'a> {
    pub fn new(out: &'a mut dyn BufMut) -> WordWriter<'a> {
        WordWriter {
            out,
            needs_space: false,
        }
    }

    pub fn maybe_add_separator(&mut self, sep: &[u8]) {
        if self.needs_space {
            self.out.put_slice(sep)
        } else {
            self.needs_space = true;
        }
    }

    pub fn maybe_add_space(&mut self) {
        self.maybe_add_separator(b" ")
    }

    pub fn write(&mut self, s: &[u8]) {
        self.maybe_add_space();
        self.out.put_slice(s);
    }
}

pub fn has_path_prefix(s: &[u8], prefix: &[u8]) -> bool {
    s.starts_with(prefix) && (s.len() == prefix.len() || s[prefix.len()..].starts_with(b"/"))
}

pub fn has_word(s: &[u8], w: &[u8]) -> bool {
    let Some(found) = memmem::find(s, w) else {
        return false;
    };
    let start = &s[..found];
    if start.last().is_some_and(|c| !is_space_byte(c)) {
        return false;
    }
    let end = &s[found + w.len()..];
    if end.first().is_some_and(|c| !is_space_byte(c)) {
        return false;
    }
    true
}

pub fn trim_prefix_str<'a>(s: &'a str, prefix: &str) -> &'a str {
    match s.strip_prefix(prefix) {
        Some(s) => s,
        None => s,
    }
}

pub fn trim_prefix<'a>(s: &'a [u8], prefix: &[u8]) -> &'a [u8] {
    match s.strip_prefix(prefix) {
        Some(s) => s,
        None => s,
    }
}

pub fn trim_suffix<'a>(s: &'a [u8], suffix: &[u8]) -> &'a [u8] {
    match s.strip_suffix(suffix) {
        Some(s) => s,
        None => s,
    }
}

#[derive(Debug)]
pub struct Pattern {
    pat: Bytes,
    percent_index: Option<usize>,
}

impl Pattern {
    pub fn new(pat: Bytes) -> Pattern {
        let idx = memchr(b'%', &pat);
        Pattern {
            pat,
            percent_index: idx,
        }
    }

    pub fn matches(&self, str: &[u8]) -> bool {
        if let Some(percent_index) = self.percent_index {
            return self.match_impl(str, percent_index);
        }
        self.pat == str
    }

    fn match_impl(&self, str: &[u8], percent_index: usize) -> bool {
        str.starts_with(&self.pat[..percent_index]) && str.ends_with(&self.pat[percent_index + 1..])
    }

    pub fn stem<'a>(&self, str: &'a [u8]) -> &'a [u8] {
        if !self.matches(str) {
            return &[];
        }
        if let Some(percent_index) = self.percent_index {
            return &str[percent_index..(str.len() - self.pat.len() + 1 + percent_index)];
        }
        &[]
    }

    pub fn append_subst(&self, s: &Bytes, subst: &Bytes) -> Bytes {
        let Some(percent_index) = self.percent_index else {
            if s == &self.pat {
                return subst.clone();
            }
            return s.clone();
        };

        if self.match_impl(s, percent_index) {
            if let Some(subst_percent_index) = memchr(b'%', subst) {
                let mut ret = BytesMut::with_capacity(subst.len() + s.len() - self.pat.len() + 1);
                ret.put_slice(&subst[..subst_percent_index]);
                ret.put_slice(&s[percent_index..(percent_index + s.len() + 1 - self.pat.len())]);
                ret.put_slice(&subst[subst_percent_index + 1..]);
                return ret.into();
            }
            return subst.clone();
        }
        s.clone()
    }

    pub fn append_subst_ref(&self, s: &Bytes, subst: &Bytes) -> Bytes {
        if self.percent_index.is_some() && subst.contains(&b'%') {
            return self.append_subst(s, subst);
        }
        let s = trim_suffix(s, &self.pat);
        let mut ret = BytesMut::with_capacity(s.len() + subst.len());
        ret.put_slice(s);
        ret.put_slice(subst);
        ret.into()
    }
}

pub fn no_line_break(s: Cow<str>) -> Cow<str> {
    if !s.contains('\n') {
        return s;
    }
    s.into_owned().replace('\n', "\\n").into()
}

pub fn trim_left_space(s: &[u8]) -> &[u8] {
    let mut s = s;
    loop {
        if s.is_empty() {
            return s;
        }
        s = s.trim_ascii_start();
        if s.starts_with(b"\\\r") || s.starts_with(b"\\\n") {
            s = &s[2..];
        } else {
            return s;
        }
    }
}

pub fn trim_right_space(s: &[u8]) -> &[u8] {
    let mut s = s;
    while let [rest @ .., last] = s {
        match last {
            b'\t' | b'\x0b' | b'\x0c' | b' ' => s = rest,
            b'\r' | b'\n' => {
                if rest.ends_with(b"\\") {
                    s = &rest[..rest.len() - 1];
                } else {
                    s = rest;
                }
            }
            _ => break,
        }
    }
    s
}

pub fn trim_space(s: &[u8]) -> &[u8] {
    trim_right_space(trim_left_space(s))
}

pub fn dirname(s: &Bytes) -> Bytes {
    let Some(found) = memrchr(b'/', s) else {
        return Bytes::from_static(b".");
    };
    if found == 0 {
        return Bytes::new();
    }
    return s.slice(..found);
}

pub fn basename(s: &[u8]) -> &[u8] {
    let Some(found) = memrchr(b'/', s) else {
        return s;
    };
    if found == 0 {
        return s;
    }
    &s[found + 1..]
}

pub fn get_ext(s: &[u8]) -> Option<&[u8]> {
    let found = memrchr(b'.', s)?;
    Some(&s[found..])
}

pub fn strip_ext(s: &[u8]) -> &[u8] {
    let Some(found) = memrchr(b'.', s) else {
        return s;
    };
    if let Some(slash_index) = memrchr(b'/', s)
        && found < slash_index
    {
        return s;
    }
    &s[0..found]
}

pub fn strip_ext_vec(mut s: Vec<u8>) -> Vec<u8> {
    let Some(found) = memrchr(b'.', &s) else {
        return s;
    };
    if let Some(slash_index) = memrchr(b'/', &s)
        && found < slash_index
    {
        return s;
    }
    s.truncate(found);
    s
}

pub fn normalize_path(mut o: &[u8]) -> Bytes {
    if o.is_empty() {
        return Bytes::new();
    }
    let mut ret = BytesMut::new();
    if o.starts_with(b"/") {
        ret.put_u8(b'/');
        o = &o[1..];
    }
    while !o.is_empty() {
        let idx = memchr(b'/', o);
        let (dir, rest) = match idx {
            Some(idx) => (&o[..idx], &o[idx + 1..]),
            None => (o, [].as_slice()),
        };
        o = rest;

        if dir == b"." || (dir == b".." && ret.as_ref() == b"/") {
            continue;
        } else if dir == b".." && !ret.is_empty() && ret.as_ref() != b".." && !ret.ends_with(b"/..")
        {
            match memrchr(b'/', ret.as_ref()) {
                Some(index) => {
                    if index == 0 {
                        ret.truncate(1);
                    } else {
                        ret.truncate(index);
                    }
                }
                None => {
                    ret.truncate(0);
                }
            }
        } else if !dir.is_empty() {
            if !ret.is_empty() && !ret.ends_with(b"/") {
                ret.put_u8(b'/');
            }
            ret.put_slice(dir);
        }
    }
    ret.into()
}

pub fn abs_path(s: &[u8]) -> Result<Bytes> {
    if s.starts_with(b"/") {
        return Ok(normalize_path(s));
    }
    let mut o = BytesMut::from(current_dir()?.as_os_str().as_bytes());
    if !s.is_empty() {
        o.put_u8(b'/');
        o.put_slice(s);
    }
    Ok(normalize_path(&o))
}

pub fn find_outside_paren(s: &[u8], pattern: &[u8]) -> Option<usize> {
    let mut prev_backslash = false;
    let mut paren_stack: Vec<u8> = Vec::new();
    let mut pattern_set = [false; 128];
    for c in pattern {
        assert!(c.is_ascii());
        pattern_set[*c as usize] = true;
    }

    for (i, c) in s.iter().enumerate() {
        if c.is_ascii() && pattern_set[*c as usize] && paren_stack.is_empty() && !prev_backslash {
            return Some(i);
        }
        match c {
            b'(' => paren_stack.push(b')'),
            b'{' => paren_stack.push(b'}'),
            b')' | b'}' => {
                if paren_stack.last() == Some(c) {
                    paren_stack.pop();
                }
            }
            _ => {}
        }
        prev_backslash = *c == b'\\' && !prev_backslash;
    }
    None
}

#[derive(Debug, PartialEq, Eq)]
pub struct EndOfLine {
    pub line: Bytes,
    pub rest: Bytes,
    pub lf_cnt: i32,
}

pub fn find_end_of_line(buf: &Bytes) -> EndOfLine {
    let mut lf_cnt = 0;
    let mut e = 0usize;
    loop {
        if e >= buf.len() {
            break;
        }
        e += skip_until2(&buf[e..], b'\n', b'\\');
        if e >= buf.len() {
            assert!(buf.len() == e);
            break;
        }
        let c = &buf[e..];
        if c.starts_with(b"\0") {
            break;
        } else if c.starts_with(b"\\") {
            if c.starts_with(b"\\\n") {
                e += 2;
                lf_cnt += 1;
            } else if c.starts_with(b"\\\r\n") {
                e += 3;
                lf_cnt += 1;
            } else if c.starts_with(b"\\\\") {
                e += 2;
            } else {
                e += 1;
            }
        } else if c.starts_with(b"\n") {
            return EndOfLine {
                line: buf.slice(..e),
                rest: buf.slice(e + 1..),
                lf_cnt: lf_cnt + 1,
            };
        }
    }
    EndOfLine {
        line: buf.slice(..e),
        rest: buf.slice(e..),
        lf_cnt,
    }
}

pub fn trim_leading_curdir(mut s: &[u8]) -> &[u8] {
    while s.starts_with(b"./") {
        s = &s[2..];
    }
    s
}

pub fn format_for_command_substitution(mut s: Vec<u8>) -> Vec<u8> {
    while s.ends_with(b"\n") {
        s.truncate(s.len() - 1)
    }
    let mut search = 0;
    while let Some(idx) = memchr(b'\n', &s[search..]) {
        search += idx;
        s[search] = b' ';
    }
    s
}

pub fn concat_dir(b: &[u8], n: &[u8]) -> Bytes {
    let mut r = BytesMut::new();
    if !b.is_empty() && !n.starts_with(b"/") {
        r.put_slice(b);
        r.put_u8(b'/');
    }
    r.put_slice(n);
    normalize_path(&r)
}

pub fn echo_escape(s: &[u8]) -> Bytes {
    let mut buf = BytesMut::new();
    for c in s {
        match c {
            b'\\' => buf.put_slice(b"\\\\\\\\"),
            b'\n' => buf.put_slice(b"\\n"),
            b'"' => buf.put_slice(b"\\\""),
            _ => buf.put_u8(*c),
        }
    }
    buf.freeze()
}

pub fn escape_shell(s: &Bytes) -> Bytes {
    let delimiters = b"\"$\\`";
    let mut prev = 0;
    let mut i = skip_until(s, delimiters);
    if i == s.len() {
        return s.clone();
    }

    let mut r = BytesMut::new();
    while i < s.len() {
        r.put_slice(&s[prev..i]);
        let c = s[i];
        r.put_u8(b'\\');
        if s[i..].starts_with(b"$$") {
            r.put_u8(b'$');
            i += 1;
        }
        r.put_u8(c);
        i += 1;
        prev = i;
        i += skip_until(&s[i..], delimiters);
    }
    r.put_slice(&s[prev..]);
    r.into()
}

pub fn is_integer(str: &[u8]) -> bool {
    if str.is_empty() {
        return false;
    }
    str.iter().all(|c| (*c as char).is_ascii_digit())
}

#[cfg(test)]
mod test {
    use super::*;

    #[test]
    fn test_word_scanner() {
        let ss = word_scanner(b"foo bar baz hogeeeeeeeeeeeeeeee").collect::<Vec<&[u8]>>();
        assert_eq!(
            ss,
            vec![
                b"foo".as_slice(),
                b"bar".as_slice(),
                b"baz".as_slice(),
                b"hogeeeeeeeeeeeeeeee".as_slice()
            ]
        );

        let ss = word_scanner(b"").collect::<Vec<&[u8]>>();
        assert!(ss.is_empty());

        let ss = word_scanner(b" a  b").collect::<Vec<&[u8]>>();
        assert_eq!(ss, vec![b"a", b"b"]);
    }

    #[test]
    fn test_has_path_prefix() {
        assert!(has_path_prefix(b"/foo/bar", b"/foo"));
        assert!(has_path_prefix(b"/foo", b"/foo"));
        assert!(!has_path_prefix(b"/foobar/baz", b"/foo"));
    }

    #[test]
    fn test_trim_prefix() {
        assert_eq!(trim_prefix(b"foo", b"foo"), b"");
        assert_eq!(trim_prefix(b"foo", b"fo"), b"o");
        assert_eq!(trim_prefix(b"foo", b""), b"foo");
        assert_eq!(trim_prefix(b"foo", b"fooo"), b"foo");
    }

    #[test]
    fn test_trim_suffix() {
        assert_eq!(trim_suffix(b"bar", b"bar"), b"");
        assert_eq!(trim_suffix(b"bar", b"ar"), b"b");
        assert_eq!(trim_suffix(b"bar", b""), b"bar");
        assert_eq!(trim_suffix(b"bar", b"bbar"), b"bar");
    }

    #[test]
    fn test_pattern_matches() {
        assert!(Pattern::new(Bytes::from_static(b"foo")).matches(b"foo"));
        assert!(Pattern::new(Bytes::from_static(b"foo%")).matches(b"foo"));
        assert!(Pattern::new(Bytes::from_static(b"foo%bar")).matches(b"foobar"));
        assert!(Pattern::new(Bytes::from_static(b"foo%bar")).matches(b"fooxbar"));
    }

    fn subst_pattern(s: &'static [u8], pat: &'static [u8], subst: &'static [u8]) -> String {
        let p = Pattern::new(Bytes::from_static(pat));
        let s = Bytes::from_static(s);
        let subst = Bytes::from_static(subst);
        String::from_utf8(p.append_subst(&s, &subst).to_vec()).unwrap()
    }

    #[test]
    fn test_subst_pattern() {
        assert_eq!(subst_pattern(b"x.c", b"%.c", b"%.o"), "x.o");
        assert_eq!(subst_pattern(b"c.x", b"c.%", b"o.%"), "o.x");
        assert_eq!(subst_pattern(b"x.c.c", b"%.c", b"%.o"), "x.c.o");
        assert_eq!(subst_pattern(b"x.x y.c", b"%.c", b"%.o"), "x.x y.o");
        assert_eq!(subst_pattern(b"x.%.c", b"%.%.c", b"OK"), "OK");
        assert_eq!(subst_pattern(b"x.c", b"x.c", b"OK"), "OK");
        assert_eq!(subst_pattern(b"x.c.c", b"x.c", b"XX"), "x.c.c");
        assert_eq!(subst_pattern(b"x.x.c", b"x.c", b"XX"), "x.x.c");
        assert_eq!(subst_pattern(b"/", b"%/", b"%"), "");
    }

    #[test]
    fn test_trim_left_space() {
        assert_eq!(trim_left_space(b" \tfoo"), b"foo");
        assert_eq!(trim_left_space(b" \\\n bar"), b"bar");
        assert_eq!(trim_left_space(b" \\a bar"), b"\\a bar");
    }

    #[test]
    fn test_no_line_break() {
        assert_eq!(no_line_break("a\nb".into()), "a\\nb");
        assert_eq!(no_line_break("a\nb\nc".into()), "a\\nb\\nc");
        assert_eq!(no_line_break("a\nb".to_string().into()), "a\\nb");
        assert_eq!(no_line_break("a\nb\nc".to_string().into()), "a\\nb\\nc");
    }

    #[test]
    fn test_has_word() {
        assert!(has_word(b"foo bar baz", b"bar"));
        assert!(has_word(b"foo bar baz", b"foo"));
        assert!(has_word(b"foo bar baz", b"baz"));
        assert!(!has_word(b"foo bar baz", b"oo"));
        assert!(!has_word(b"foo bar baz", b"ar"));
        assert!(!has_word(b"foo bar baz", b"ba"));
        assert!(!has_word(b"foo bar baz", b"az"));
        assert!(!has_word(b"foo bar baz", b"ba"));
        assert!(!has_word(b"foo bar baz", b"fo"));
    }

    #[test]
    fn test_normalize_path() {
        assert_eq!(normalize_path(b""), "");
        assert_eq!(normalize_path(b"."), "");
        assert_eq!(normalize_path(b"/"), "/");
        assert_eq!(normalize_path(b"/tmp"), "/tmp");
        assert_eq!(normalize_path(b"////tmp////"), "/tmp");
        assert_eq!(normalize_path(b"a////b"), "a/b");
        assert_eq!(normalize_path(b"a//.//b"), "a/b");
        assert_eq!(normalize_path(b"a////b//../c/////"), "a/c");
        assert_eq!(normalize_path(b"../foo"), "../foo");
        assert_eq!(normalize_path(b"./foo"), "foo");
        assert_eq!(normalize_path(b"x/y/..//../foo"), "foo");
        assert_eq!(normalize_path(b"x/../../foo"), "../foo");
        assert_eq!(normalize_path(b"/../foo"), "/foo");
        assert_eq!(normalize_path(b"/../../foo"), "/foo");
        assert_eq!(normalize_path(b"/a/.."), "/");
        assert_eq!(normalize_path(b"/a/../../foo"), "/foo");
        assert_eq!(normalize_path(b"/a/b/.."), "/a");
        assert_eq!(normalize_path(b"../../a/b"), "../../a/b");
        assert_eq!(normalize_path(b"../../../a/b"), "../../../a/b");
        assert_eq!(normalize_path(b".././../a/b"), "../../a/b");
        assert_eq!(normalize_path(b"./../../a/b"), "../../a/b");
    }

    #[test]
    fn test_find_end_of_line() {
        assert_eq!(
            find_end_of_line(&Bytes::from_static(b"foo")),
            EndOfLine {
                line: Bytes::from_static(b"foo"),
                rest: Bytes::from_static(b""),
                lf_cnt: 0
            }
        );
        assert_eq!(
            find_end_of_line(&Bytes::from_static(b"foo\nbar")),
            EndOfLine {
                line: Bytes::from_static(b"foo"),
                rest: Bytes::from_static(b"bar"),
                lf_cnt: 1
            }
        );
        assert_eq!(
            find_end_of_line(&Bytes::from_static(b"foo\\\nbar\nbaz")),
            EndOfLine {
                line: Bytes::from_static(b"foo\\\nbar"),
                rest: Bytes::from_static(b"baz"),
                lf_cnt: 2
            }
        );

        assert_eq!(
            find_end_of_line(&Bytes::from_static(b"a\\")),
            EndOfLine {
                line: Bytes::from_static(b"a\\"),
                rest: Bytes::from_static(b""),
                lf_cnt: 0
            }
        );
    }

    #[test]
    fn test_is_integer() {
        assert!(is_integer(b"0"));
        assert!(is_integer(b"9"));
        assert!(is_integer(b"1234"));
        assert!(!is_integer(b""));
        assert!(!is_integer(b"a234"));
        assert!(!is_integer(b"123a"));
        assert!(!is_integer(b"12a4"));
    }

    #[test]
    fn test_find_outside_paren_simple() {
        assert_eq!(find_outside_paren(b"abc", b"b"), Some(1));
        assert_eq!(find_outside_paren(b":abc", b":"), Some(0));
        assert_eq!(find_outside_paren(b"abc:", b":"), Some(3));
        assert_eq!(find_outside_paren(b"abc", b"d"), None);
        assert_eq!(find_outside_paren(b"", b"a"), None);
        assert_eq!(find_outside_paren(b"abc", &[]), None);
        assert_eq!(find_outside_paren(b"a=b:c", b":="), Some(1)); // Finds '=' first
        assert_eq!(find_outside_paren(b"a:b=c", b":="), Some(1)); // Finds ':' first
    }

    #[test]
    fn test_find_outside_paren_parentheses() {
        assert_eq!(find_outside_paren(b"a(b:c)d", b":"), None); // ':' is inside ()
        assert_eq!(find_outside_paren(b"a{b:c}d", b":"), None); // ':' is inside {}
        assert_eq!(find_outside_paren(b"a(b)c:d", b":"), Some(5)); // ':' is outside after ()
        assert_eq!(find_outside_paren(b"a{b}c:d", b":"), Some(5)); // ':' is outside after {}
        assert_eq!(find_outside_paren(b"a:b(c)d", b":"), Some(1)); // ':' is outside before ()
        assert_eq!(find_outside_paren(b"a((b:c))d", b":"), None); // Nested ()
        assert_eq!(find_outside_paren(b"a{{b:c}}d", b":"), None); // Nested {}
        assert_eq!(find_outside_paren(b"a({b:c})d", b":"), None); // Mixed nested {} inside ()
        assert_eq!(find_outside_paren(b"a{(b:c)}d", b":"), None); // Mixed nested () inside {}
        assert_eq!(find_outside_paren(b"a(b):c", b":"), Some(4)); // Immediately after )
        assert_eq!(find_outside_paren(b"a{b}:c", b":"), Some(4)); // Immediately after }
        // Mismatched parens - should still find outside ones correctly before mismatch
        assert_eq!(find_outside_paren(b"a(b:c", b":"), None);
        assert_eq!(find_outside_paren(b"a)b:c", b":"), Some(3));
        assert_eq!(find_outside_paren(b"a{b:c", b":"), None);
        assert_eq!(find_outside_paren(b"a}b:c", b":"), Some(3));
    }

    #[test]
    fn test_find_outside_paren_escapes() {
        assert_eq!(find_outside_paren(b"a\\:b:c", b":"), Some(4)); // Escaped ':' ignored, finds second ':'
        assert_eq!(find_outside_paren(b"a\\\\:b", b":"), Some(3)); // Escaped '\\', finds ':'
        // Test case for escaped newline - find_outside_paren doesn't see the newline itself
        // as it's processed line by line after find_end_of_line.
        // Let's test a scenario where the escape is just before the target.
        assert_eq!(find_outside_paren(b"abc\\", b"c"), Some(2)); // Escaped char at end
        assert_eq!(find_outside_paren(b"abc\\:", b":"), None); // Escaped target char
    }

    #[test]
    fn test_find_outside_paren_combinations() {
        assert_eq!(find_outside_paren(b"a(b\\:c):d", b":"), Some(7)); // Escaped ':' inside (), find ':' outside
    }
}
