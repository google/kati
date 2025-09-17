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

use std::sync::Arc;

use anyhow::Result;
use bytes::{BufMut, Bytes, BytesMut};
use memchr::memchr;

use crate::eval::{Evaluator, FrameType};
use crate::flags::FLAGS;
use crate::func::{FuncInfo, get_func_info};
use crate::loc::Loc;
use crate::strutil::{Pattern, WordWriter, trim_right_space, trim_suffix, word_scanner};
use crate::symtab::{Symbol, intern};
use crate::{error_loc, kati_warn_loc, log};

pub trait Evaluable {
    fn eval(&self, ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()>;

    fn eval_to_buf_mut(&self, ev: &mut Evaluator) -> Result<BytesMut> {
        let mut out = BytesMut::new();
        self.eval(ev, &mut out)?;
        Ok(out)
    }

    fn eval_to_buf(&self, ev: &mut Evaluator) -> Result<Bytes> {
        Ok(self.eval_to_buf_mut(ev)?.freeze())
    }

    // Whether this Evaluable is either knowably a function (e.g. one of the
    // built-ins) or likely to be a function-type macro (i.e. one that has
    // positional $(1) arguments to be expanded inside it. However, this is
    // only a heuristic guess. In order to not actually evaluate the expression,
    // because doing so could have side effects like calling $(error ...) or
    // doing a nested eval that assigns variables, we don't handle the case where
    // the variable name is itself a variable expansion inside a deferred
    // expansion variable, and return true in that case. Implementations of this
    // function must also not mark variables as used, as that can trigger unwanted
    // warnings. They should use ev->PeekVar().
    fn is_func(&self) -> bool;
}

#[derive(PartialEq, Eq, Clone, Copy, Debug)]
pub enum ParseExprOpt {
    Normal,
    Define,
    Command,
    Func,
}

#[derive(Debug, PartialEq)]
pub enum Value {
    Literal(Option<Loc>, Bytes),
    List(Option<Loc>, Vec<Arc<Value>>),
    SymRef(Loc, Symbol),
    VarRef(Loc, Arc<Value>),
    VarSubst {
        loc: Loc,
        name: Arc<Value>,
        pat: Arc<Value>,
        subst: Arc<Value>,
    },
    Func {
        loc: Loc,
        fi: &'static FuncInfo,
        args: Vec<Arc<Value>>,
    },
}

impl Evaluable for Value {
    fn eval(&self, ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
        match self {
            Value::Literal(_, lit) => out.put_slice(lit),
            Value::List(_, vec) => {
                for v in vec {
                    v.eval(ev, out)?;
                }
            }
            Value::SymRef(_, sym) => {
                let sym = *sym;
                if let Some(v) = ev.lookup_var_for_eval(sym)? {
                    let v = v.read();
                    v.used(ev, &sym)?;
                    v.eval(ev, out)?;
                    v.check_current_referencing_file(&ev.loc, sym)?;
                    ev.var_eval_complete(sym);
                }
            }
            Value::VarRef(_, var) => {
                ev.eval_depth += 1;
                let name = var.eval_to_buf(ev)?;
                ev.eval_depth -= 1;
                let sym = intern(name);
                if let Some(v) = ev.lookup_var_for_eval(sym)? {
                    let v = v.read();
                    v.used(ev, &sym)?;
                    v.eval(ev, out)?;
                    v.check_current_referencing_file(&ev.loc, sym)?;
                    ev.var_eval_complete(sym);
                }
            }
            Value::VarSubst {
                loc: _,
                name,
                pat,
                subst,
            } => {
                ev.eval_depth += 1;
                let name = name.eval_to_buf(ev)?;
                let sym = intern(name);
                let v = ev.lookup_var(sym)?;
                let pat_str = pat.eval_to_buf(ev)?;
                let subst = subst.eval_to_buf(ev)?;
                ev.eval_depth -= 1;
                if let Some(v) = v {
                    let v = v.read();
                    v.used(ev, &sym)?;
                    let value = v.eval_to_buf(ev)?;
                    let mut ww = WordWriter::new(out);
                    let pat = Pattern::new(pat_str);
                    for tok in word_scanner(&value) {
                        ww.maybe_add_space();
                        let tok = value.slice_ref(tok);
                        ww.out.put_slice(&pat.append_subst_ref(&tok, &subst));
                    }
                }
            }
            Value::Func { loc, fi, args } => {
                let _frame = ev.enter(FrameType::FunCall, Bytes::from_static(fi.name), loc.clone());
                log!(
                    "Invoke func {}({:?})",
                    String::from_utf8_lossy(fi.name),
                    args
                );
                ev.eval_depth += 1;
                (fi.func)(args, ev, out)?;
                ev.eval_depth -= 1;
            }
        }
        Ok(())
    }

    fn is_func(&self) -> bool {
        match self {
            Value::Func { .. } => true,
            Value::List(_, list) => list.iter().any(|v| v.is_func()),
            Value::SymRef(_, sym) => {
                // This is a heuristic, where say that if a variable has positional
                // parameters, we think it is likely to be a function. Callers can use
                // .KATI_SYMBOLS to extract variables and their values, without evaluating
                // macros that are likely to have side effects.
                crate::strutil::is_integer(&sym.as_bytes())
            }
            Value::VarRef(_, _) => {
                // This is the unhandled edge case as described in the Evaluable::is_func
                true
            }
            Value::VarSubst {
                name, pat, subst, ..
            } => name.is_func() || pat.is_func() || subst.is_func(),
            Value::Literal(_, _) => false,
        }
    }
}

impl Value {
    pub fn loc(&self) -> Option<Loc> {
        match self {
            Value::Literal(loc, _) => loc.clone(),
            Value::List(loc, _) => loc.clone(),
            Value::SymRef(loc, _) => Some(loc.clone()),
            Value::VarRef(loc, _) => Some(loc.clone()),
            Value::VarSubst { loc, .. } => Some(loc.clone()),
            Value::Func { loc, .. } => Some(loc.clone()),
        }
    }
}

fn close_paren(c: u8) -> Option<u8> {
    match c {
        b'(' => Some(b')'),
        b'{' => Some(b'}'),
        _ => None,
    }
}

fn should_handle_comments(opt: ParseExprOpt) -> bool {
    !matches!(opt, ParseExprOpt::Define | ParseExprOpt::Command)
}

fn skip_spaces(loc: &mut Loc, s: &[u8], terms: &[u8]) -> usize {
    let mut i = 0;
    while i < s.len() {
        let remaining = &s[i..];
        let c = remaining[0];
        if terms.contains(&c) {
            return i;
        }

        if !c.is_ascii_whitespace() {
            if !remaining.starts_with(b"\\\r") && !remaining.starts_with(b"\\\n") {
                return i;
            }

            loc.line += 1; // This is a backspace continuation
        }
        i += 1;
    }
    s.len()
}

fn parse_func(
    loc: &mut Loc,
    fi: &FuncInfo,
    s: Bytes,
    mut i: usize,
    mut terms: Vec<u8>,
) -> Result<(usize, Vec<Arc<Value>>)> {
    let start_loc = loc.clone();
    terms.truncate(2);
    terms[1] = b',';
    i += skip_spaces(loc, &s[i..], &terms);
    if i == s.len() {
        return Ok((i, vec![]));
    }

    let mut nargs = 1;
    let mut args = Vec::new();
    loop {
        if fi.arity > 0 && nargs >= fi.arity {
            terms.truncate(1); // Drop ','.
        }

        if fi.trim_space {
            while i < s.len() {
                let c = s[i];
                if c.is_ascii_whitespace() {
                    i += 1;
                    continue;
                }

                let t = &s[i..];
                if t.starts_with(b"\\\r") || t.starts_with(b"\\\n") {
                    loc.line += 1;
                    i += 1;
                    continue;
                }

                break;
            }
        }

        let trim_right_space = fi.trim_space || (nargs == 1 && fi.trim_right_space_1st);
        let (n, val) = parse_expr_impl(
            loc,
            s.slice(i..),
            Some(&terms),
            ParseExprOpt::Func,
            trim_right_space,
        )?;
        // TODO: concatLine???
        args.push(val);
        i += n;
        if i == s.len() {
            error_loc!(
                Some(&start_loc),
                "*** unterminated call to function '{}': missing '{}'.",
                String::from_utf8_lossy(fi.name),
                char::from(terms[0])
            );
        }
        nargs += 1;
        if s[i] == terms[0] {
            i += 1;
            break;
        }
        i += 1; // Should be ','.
        if i == s.len() {
            break;
        }
    }

    if nargs <= fi.min_arity {
        error_loc!(
            Some(&start_loc),
            "*** insufficient number of arguments ({}) to function `{}'.",
            nargs - 1,
            String::from_utf8_lossy(fi.name)
        );
    }

    Ok((i, args))
}

fn parse_dollar(loc: &mut Loc, s: Bytes, end_paren: bool) -> Result<(usize, Arc<Value>)> {
    assert!(s.len() >= 2);
    assert!(s.starts_with(b"$"));
    assert!(!s.starts_with(b"$$"));

    let start_loc = loc.clone();

    let Some(cp) = close_paren(s[1]) else {
        return Ok((
            2,
            Arc::new(Value::SymRef(start_loc.clone(), intern(s.slice(1..2)))),
        ));
    };

    let mut terms = vec![cp, b':', b' '];
    let mut i = 2;
    loop {
        let (n, vname) =
            parse_expr_impl(loc, s.slice(i..), Some(&terms), ParseExprOpt::Normal, false)?;
        i += n;

        let t: &[u8] = &s[i..];
        if t.first() == Some(&cp) || (end_paren && t.is_empty() && cp == b')') {
            if let Value::Literal(_, lit) = &*vname {
                let sym = intern(lit.clone());
                if FLAGS.enable_kati_warnings
                    && let Some(found) = sym.to_string().find([' ', '(', '{'])
                {
                    kati_warn_loc!(
                        Some(&start_loc),
                        "*warning*: variable lookup with '{}': {}",
                        &sym.to_string()[found..found + 1],
                        String::from_utf8_lossy(&s)
                    )
                }
                return Ok((i + 1, Arc::new(Value::SymRef(start_loc, sym))));
            }
            return Ok((i + 1, Arc::new(Value::VarRef(start_loc, vname))));
        }

        if t.first() == Some(&b' ') || t.first() == Some(&b'\\') {
            // ${func ...}
            if let Value::Literal(_, lit) = &*vname {
                if let Some(fi) = get_func_info(lit) {
                    let (idx, args) = parse_func(loc, fi, s, i + 1, terms)?;
                    return Ok((
                        idx,
                        Arc::new(Value::Func {
                            loc: start_loc,
                            fi,
                            args,
                        }),
                    ));
                } else {
                    kati_warn_loc!(
                        Some(&start_loc),
                        "*warning*: unknown make function {lit:?}: {}",
                        String::from_utf8_lossy(&s)
                    );
                }
            }

            // Not a function. Drop ' ' from |terms| and parse it
            // again. This is inefficient, but this code path should be
            // rarely used.
            terms.truncate(2);
            i = 2;
            continue;
        }

        if t.first() == Some(&b':') {
            terms.truncate(2);
            terms[1] = b'=';
            let (n, pat) = parse_expr_impl(
                loc,
                s.slice(i + 1..),
                Some(&terms),
                ParseExprOpt::Normal,
                false,
            )?;
            i += 1 + n;
            if s.get(i) == Some(&cp) {
                return Ok((
                    i + 1,
                    Arc::new(Value::VarRef(
                        start_loc.clone(),
                        Arc::new(Value::List(
                            Some(start_loc),
                            vec![
                                vname,
                                Arc::new(Value::Literal(None, Bytes::from_static(b":"))),
                                pat,
                            ],
                        )),
                    )),
                ));
            }

            terms.truncate(1);
            let (n, subst) = parse_expr_impl(
                loc,
                s.slice(i + 1..),
                Some(&terms),
                ParseExprOpt::Normal,
                false,
            )?;
            i += 1 + n;
            return Ok((
                i + 1,
                Arc::new(Value::VarSubst {
                    loc: start_loc,
                    name: vname,
                    pat,
                    subst,
                }),
            ));
        }

        // GNU make accepts expressions like $((). See unmatched_paren*.mk
        // for detail.
        if let Some(found) = memchr(cp, &s) {
            kati_warn_loc!(
                Some(&start_loc),
                "*warning*: unmatched parentheses: {}",
                String::from_utf8_lossy(&s)
            );
            return Ok((
                s.len(),
                Arc::new(Value::SymRef(start_loc.clone(), intern(s.slice(2..found)))),
            ));
        }

        error_loc!(Some(&start_loc), "*** unterminated variable reference.");
    }
}

pub fn parse_expr_impl(
    loc: &mut Loc,
    s: Bytes,
    terms: Option<&[u8]>,
    opt: ParseExprOpt,
    trim_right_sp: bool,
) -> Result<(usize, Arc<Value>)> {
    parse_expr_impl_ext(loc, s, terms, opt, trim_right_sp, false)
}

pub fn parse_expr_impl_ext(
    loc: &mut Loc,
    s: Bytes,
    terms: Option<&[u8]>,
    opt: ParseExprOpt,
    trim_right_sp: bool,
    // This is for compatibility with a read-past-end in ckati
    end_paren: bool,
) -> Result<(usize, Arc<Value>)> {
    let list_loc = loc.clone();

    let s = s.slice_ref(trim_suffix(&s, b"\r"));

    let mut b = 0usize;
    let mut save_paren: Option<u8> = None;
    let mut paren_depth: i32 = 0;
    let mut i = 0usize;
    let mut list: Vec<Arc<Value>> = Vec::new();
    let mut terms_ignored = 0;

    while i < s.len() {
        let item_loc = loc.clone();

        let remaining = &s[i..];
        let c = remaining[0];
        if let Some(terms) = terms
            && save_paren.is_none()
            && terms[terms_ignored..].contains(&c)
        {
            break;
        }

        // Handle a comment
        if terms.is_none() && c == b'#' && should_handle_comments(opt) {
            if i > b {
                list.push(Arc::new(Value::Literal(None, s.slice(b..i))));
            }
            let mut was_backslash = false;
            while i < s.len() && s[i] != b'\n' || was_backslash {
                was_backslash = !was_backslash && s[i] == b'\\';
                i += 1;
            }
            if list.len() == 1 {
                return Ok((i, list.pop().unwrap()));
            }
            return Ok((i, Arc::new(Value::List(Some(item_loc), list))));
        }

        if c == b'$' {
            if i + 1 >= s.len() {
                break;
            }

            if i > b {
                list.push(Arc::new(Value::Literal(None, s.slice(b..i))));
            }

            if remaining.starts_with(b"$$") {
                list.push(Arc::new(Value::Literal(None, Bytes::from_static(b"$"))));
                i += 2;
                b = i;
                continue;
            }

            if let Some(terms) = terms
                && terms[terms_ignored..].contains(&remaining[1])
            {
                let val = Arc::new(Value::Literal(None, Bytes::from_static(b"$")));
                if list.is_empty() {
                    return Ok((i + 1, val));
                }
                list.push(val);
                return Ok((i + 1, Arc::new(Value::List(Some(item_loc), list))));
            }

            let (n, v) = parse_dollar(loc, s.slice(i..), end_paren)?;
            list.push(v);
            i += n;
            b = i;
            continue;
        }

        if (c == b'(' || c == b'{') && opt == ParseExprOpt::Func {
            let cp = close_paren(c);
            if terms
                .map(|v| v[terms_ignored..].first() == cp.as_ref())
                .unwrap_or(false)
            {
                paren_depth += 1;
                save_paren = cp;
                terms_ignored += 1;
            } else if cp == save_paren {
                paren_depth += 1;
            }
            i += 1;
            continue;
        }

        if Some(c) == save_paren {
            paren_depth -= 1;
            if paren_depth == 0 {
                terms_ignored -= 1;
                save_paren = None;
            }
        }

        if c == b'\\' && i + 1 < s.len() && opt != ParseExprOpt::Command {
            let n = remaining[1];
            if n == b'\\' {
                i += 2;
                continue;
            }
            if n == b'#' && should_handle_comments(opt) {
                list.push(Arc::new(Value::Literal(None, s.slice(b..i))));
                i += 1;
                b = i;
                i += 1;
                continue;
            }
            if n == b'\r' || n == b'\n' {
                loc.line += 1;
                if let Some(terms) = terms
                    && terms.contains(&b' ')
                {
                    break;
                }
                if i > b {
                    list.push(Arc::new(Value::Literal(
                        None,
                        s.slice_ref(trim_right_space(&s[b..i])),
                    )));
                }
                list.push(Arc::new(Value::Literal(None, Bytes::from_static(b" "))));
                // Skip the current escaped newline
                i += 2;
                if n == b'\r' && i < s.len() && s[i] == b'\n' {
                    i += 1;
                }
                // Then continue skipping escaped newlines, spaces, and tabs
                while i < s.len() {
                    let t = &s[i..];
                    if t.starts_with(b"\\\r") || t.starts_with(b"\\\n") {
                        loc.line += 1;
                        i += 2;
                        continue;
                    }
                    if !(t[0] == b' ' || t[0] == b'\t') {
                        break;
                    }
                    i += 1;
                }
                b = i;
                i -= 1;
            }
        }

        i += 1;
    }

    if i > b {
        let mut rest = &s[b..i];
        if trim_right_sp {
            rest = trim_right_space(rest);
        }
        if !rest.is_empty() {
            list.push(Arc::new(Value::Literal(None, s.slice_ref(rest))))
        }
    }
    if list.len() == 1 {
        Ok((i, list.pop().unwrap()))
    } else {
        Ok((i, Arc::new(Value::List(Some(list_loc), list))))
    }
}

pub fn parse_expr(loc: &mut Loc, s: Bytes, opt: ParseExprOpt) -> Result<Arc<Value>> {
    let (_i, val) = parse_expr_impl(loc, s, None, opt, false)?;
    Ok(val)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_expr() {
        assert_eq!(
            parse_expr(
                &mut Loc::default(),
                Bytes::from_static(b"foo"),
                ParseExprOpt::Normal
            )
            .unwrap(),
            Arc::new(Value::Literal(None, Bytes::from_static(b"foo")))
        );
        assert_eq!(
            parse_expr(
                &mut Loc::default(),
                Bytes::from_static(b"$(foo)"),
                ParseExprOpt::Normal
            )
            .unwrap(),
            Arc::new(Value::SymRef(Loc::default(), intern("foo")))
        );
    }

    #[test]
    fn test_eval_define_simplified() {
        let s = Bytes::from_static(b"$(eval dst := $$(notdir $$(src)))");
        assert_eq!(
            parse_expr(&mut Loc::default(), s, ParseExprOpt::Define).unwrap(),
            Arc::new(Value::Func {
                loc: Loc::default(),
                fi: get_func_info(b"eval").unwrap(),
                args: vec![Arc::new(Value::List(
                    Some(Loc::default()),
                    vec![
                        Arc::new(Value::Literal(None, Bytes::from_static(b"dst := "))),
                        Arc::new(Value::Literal(None, Bytes::from_static(b"$"))),
                        Arc::new(Value::Literal(None, Bytes::from_static(b"(notdir "))),
                        Arc::new(Value::Literal(None, Bytes::from_static(b"$"))),
                        Arc::new(Value::Literal(None, Bytes::from_static(b"(src))"))),
                    ]
                ))],
            })
        )
    }

    #[test]
    fn test_parse_dollar() {
        assert_eq!(
            parse_dollar(&mut Loc::default(), Bytes::from_static(b"${foo}bar"), false).unwrap(),
            (6, Arc::new(Value::SymRef(Loc::default(), intern("foo"))))
        );
        assert_eq!(
            parse_dollar(
                &mut Loc::default(),
                Bytes::from_static(b"$(info ***   - Re-execute)"),
                false,
            )
            .unwrap(),
            (
                26,
                Arc::new(Value::Func {
                    loc: Loc::default(),
                    fi: get_func_info(b"info").unwrap(),
                    args: vec![Arc::new(Value::Literal(
                        None,
                        Bytes::from_static(b"***   - Re-execute")
                    ))],
                })
            )
        );
        assert_eq!(
            parse_dollar(
                &mut Loc::default(),
                Bytes::from_static(b"$(info ***   - Re-execute envsetup (\". envsetup.sh\"))"),
                false,
            )
            .unwrap(),
            (
                53,
                Arc::new(Value::Func {
                    loc: Loc::default(),
                    fi: get_func_info(b"info").unwrap(),
                    args: vec![Arc::new(Value::Literal(
                        None,
                        Bytes::from_static(b"***   - Re-execute envsetup (\". envsetup.sh\")")
                    ))],
                })
            )
        );
    }

    #[test]
    fn test_call_func() {
        assert_eq!(
            parse_expr(
                &mut Loc::default(),
                Bytes::from_static(b"$(call to-lower,$(upper))"),
                ParseExprOpt::Normal
            )
            .unwrap(),
            Arc::new(Value::Func {
                loc: Loc::default(),
                fi: get_func_info(b"call").unwrap(),
                args: vec![
                    Arc::new(Value::Literal(None, Bytes::from_static(b"to-lower"))),
                    Arc::new(Value::SymRef(Loc::default(), intern("upper"))),
                ],
            })
        )
    }

    #[test]
    fn test_subst2() {
        assert_eq!(
            parse_expr(
                &mut Loc::default(),
                Bytes::from_static(b"$(subst $(space),$,,$(foo))"),
                ParseExprOpt::Normal
            )
            .unwrap(),
            Arc::new(Value::Func {
                loc: Loc::default(),
                fi: get_func_info(b"subst").unwrap(),
                args: vec![
                    Arc::new(Value::SymRef(Loc::default(), intern("space"))),
                    Arc::new(Value::Literal(None, Bytes::from_static(b"$"))),
                    Arc::new(Value::List(
                        Some(Loc::default()),
                        vec![
                            Arc::new(Value::Literal(None, Bytes::from_static(b","))),
                            Arc::new(Value::SymRef(Loc::default(), intern("foo"))),
                        ]
                    )),
                ],
            })
        )
    }

    #[test]
    fn test_ckati_end_paren() {
        // ckati does not error on lines like `ifeq (foo,$(BAR)` as parse_expr
        // gets `$(BAR`, but reads off the end of the string view to find the
        // ending `)`.
        let mut loc = Loc::default();
        assert_eq!(
            parse_expr_impl_ext(
                &mut loc,
                Bytes::from_static(b"$(BAR"),
                None,
                ParseExprOpt::Normal,
                false,
                false
            )
            .unwrap_err()
            .to_string(),
            "<unknown>:0: *** unterminated variable reference."
        );
        assert_eq!(
            parse_expr_impl_ext(
                &mut loc,
                Bytes::from_static(b"$(BAR"),
                None,
                ParseExprOpt::Normal,
                false,
                true
            )
            .unwrap(),
            (6, Arc::new(Value::SymRef(loc, intern("BAR"))))
        );
    }
}
