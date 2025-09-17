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
use bytes::{Buf, Bytes};
use memchr::{memchr, memchr3};
use parking_lot::Mutex;

use crate::{
    collect_stats, error_loc,
    expr::{ParseExprOpt, Value, parse_expr, parse_expr_impl, parse_expr_impl_ext},
    loc::Loc,
    stmt::{
        AssignDirective, AssignOp, AssignStmt, CommandStmt, CondOp, ExportStmt, IfStmt,
        IncludeStmt, RuleSep, RuleStmt, Stmt,
    },
    strutil::{
        find_end_of_line, find_outside_paren, trim_left_space, trim_right_space, trim_space,
    },
    symtab::Symbol,
    warn_loc,
};

struct IfState {
    stmt: Arc<IfStmt>,
    is_in_else: bool,
    num_nest: i32,
}

struct Parser {
    buf: Bytes,
    l: usize,
    // Represents if we just parsed a rule or an expression.
    // Expressions are included because they can expand into
    // a rule, see testcase/rule_in_var.mk.
    after_rule: bool,

    stmts: Arc<Mutex<Vec<Stmt>>>,
    out_stmts: Arc<Mutex<Vec<Stmt>>>,

    define_name: Option<Bytes>,
    num_define_nest: i32,
    define_start: usize,
    define_start_line: i32,

    orig_line_with_directives: Option<Bytes>,
    current_directive: Option<AssignDirective>,

    num_if_nest: i32,
    if_stack: Vec<IfState>,

    loc: Loc,
    fixed_lineno: bool,
}

impl Parser {
    fn with_buf(buf: &Bytes, loc: Loc, stmts: Arc<Mutex<Vec<Stmt>>>, fixed_lineno: bool) -> Self {
        Self {
            buf: buf.clone(),
            l: 0,
            after_rule: false,

            stmts: stmts.clone(),
            out_stmts: stmts,

            define_name: None,
            num_define_nest: 0,
            define_start: 0,
            define_start_line: 0,

            orig_line_with_directives: None,
            current_directive: None,

            num_if_nest: 0,
            if_stack: Vec::new(),

            loc,
            fixed_lineno,
        }
    }

    fn parse(&mut self) -> Result<()> {
        self.l = 0;
        let buf = self.buf.clone();

        while self.l < buf.len() {
            let eol = find_end_of_line(&buf.slice(self.l..));
            let new_l = self.l + eol.line.len();
            if !self.fixed_lineno {
                self.loc.line += 1;
            }
            let mut line = eol.line;
            if line.ends_with(b"\r") {
                line.truncate(line.len() - 1);
            }
            self.orig_line_with_directives = Some(line.clone());
            self.parse_line(line)?;
            if !self.fixed_lineno {
                self.loc.line += eol.lf_cnt - 1;
            }
            if new_l == buf.len() {
                break;
            }
            self.l = new_l + 1
        }

        if !self.if_stack.is_empty() {
            let mut loc = self.loc.clone();
            loc.line += 1;
            error_loc!(Some(&loc), "*** missing `endif'.");
        }
        if self.define_name.is_some() {
            let mut loc = self.loc.clone();
            loc.line = self.define_start_line;
            error_loc!(Some(&loc), "*** missing `endef', unterminated `define'.",);
        }

        Ok(())
    }

    fn parse_line(&mut self, line: Bytes) -> Result<()> {
        if self.define_name.is_some() {
            return self.parse_inside_define(line);
        }

        if line.is_empty() || &*line == b"\r" {
            return Ok(());
        }

        self.current_directive = None;

        if line.starts_with(b"\t") && self.after_rule {
            let loc = self.loc.clone();
            let mut mutable_loc = self.loc.clone();
            let expr = parse_expr(&mut mutable_loc, line.slice(1..), ParseExprOpt::Command)?;
            self.out_stmts
                .lock()
                .push(CommandStmt::new(loc, line, expr));
            return Ok(());
        }

        let line = line.slice_ref(trim_left_space(&line));

        if line.starts_with(b"#") {
            return Ok(());
        }

        if self.handle_make_directive(&line)? {
            return Ok(());
        }

        self.parse_rule_or_assign(line)
    }

    fn parse_rule_or_assign(&mut self, line: Bytes) -> Result<()> {
        let Some(sep) = find_outside_paren(line.as_ref(), b":=;") else {
            return self.parse_rule(line, None);
        };
        let s = &line[sep..];
        if s.starts_with(b";") {
            return self.parse_rule(line, None);
        } else if s.starts_with(b"=") {
            return self.parse_assign(line, sep);
        } else if s[1..].starts_with(b"=") {
            return self.parse_assign(line, sep + 1);
        } else if s.starts_with(b":") {
            return self.parse_rule(line, Some(sep));
        }
        unreachable!()
    }

    fn parse_rule(&mut self, line: Bytes, mut sep: Option<usize>) -> Result<()> {
        let orig_line = self.orig_line_with_directives.clone().unwrap();
        let mut line = line;
        if self.current_directive.is_some() {
            if self.is_in_export() {
                return Ok(());
            }
            if let Some(sep) = sep.as_mut() {
                *sep += orig_line.len() - line.len()
            }
            line = orig_line.clone();
        }

        line = line.slice_ref(trim_left_space(&line));
        if line.is_empty() {
            return Ok(());
        }

        if orig_line.starts_with(b"\t") {
            error_loc!(
                Some(&self.loc),
                "*** commands commence before first target."
            );
        }

        let rule_loc = self.loc.clone();
        let rule_lhs: Arc<Value>;
        let mut rule_sep = RuleSep::Null;
        let rule_rhs: Option<Arc<Value>>;

        let sep_plus_one = sep.map(|sep| sep + 1).unwrap_or(0);

        let found = find_outside_paren(&line[sep_plus_one..], b"=;");
        let mut mutable_loc = self.loc.clone();
        if let Some(mut found) = found {
            found += sep_plus_one;
            rule_lhs = parse_expr(
                &mut mutable_loc,
                line.slice_ref(trim_space(&line[..found])),
                ParseExprOpt::Normal,
            )?;
            if line[found..].starts_with(b";") {
                rule_sep = RuleSep::Semicolon;
            } else if line[found..].starts_with(b"=$=") {
                rule_sep = RuleSep::FinalEq;
                found += 2;
            } else if line[found..].starts_with(b"=") {
                rule_sep = RuleSep::Eq;
            }
            let opt = match rule_sep {
                RuleSep::Semicolon => ParseExprOpt::Command,
                _ => ParseExprOpt::Normal,
            };
            rule_rhs = Some(parse_expr(
                &mut mutable_loc,
                line.slice_ref(trim_left_space(&line[found + 1..])),
                opt,
            )?);
        } else {
            rule_lhs = parse_expr(&mut mutable_loc, line, ParseExprOpt::Normal)?;
            rule_rhs = None;
        }
        self.after_rule = true;
        self.out_stmts
            .lock()
            .push(RuleStmt::new(rule_loc, rule_lhs, rule_sep, rule_rhs));
        Ok(())
    }

    fn parse_assign(&mut self, line: Bytes, separator_pos: usize) -> Result<()> {
        if separator_pos == 0 {
            error_loc!(Some(&self.loc), "*** empty variable name ***");
        }
        let mut assign = parse_assign_statement(&line, separator_pos);

        // If rhs starts with '$=', this is 'final assignment',
        // e.g., a combination of the assignment and
        //  .KATI_READONLY := <lhs>
        // statement. Note that we assume that ParseAssignStatement
        // trimmed the left
        let is_final = assign.rhs.starts_with(b"$=");
        if is_final {
            assign.rhs = trim_left_space(&assign.rhs[2..]);
        }

        let assign_loc = self.loc.clone();
        let mut mutable_loc = self.loc.clone();
        let lhs = parse_expr(
            &mut mutable_loc,
            line.slice_ref(assign.lhs),
            ParseExprOpt::Normal,
        )?;
        let orig_rhs = line.slice_ref(assign.rhs);
        let rhs = parse_expr(&mut mutable_loc, orig_rhs.clone(), ParseExprOpt::Normal)?;

        self.after_rule = false;
        self.out_stmts.lock().push(AssignStmt::new(
            assign_loc,
            lhs,
            rhs,
            orig_rhs,
            assign.op,
            self.current_directive,
            is_final,
        ));
        Ok(())
    }

    fn parse_include(&mut self, line: Bytes, directive: &[u8]) -> Result<()> {
        let loc = self.loc.clone();
        let mut mutable_loc = loc.clone();
        let expr = parse_expr(&mut mutable_loc, line, ParseExprOpt::Normal)?;
        self.out_stmts
            .lock()
            .push(IncludeStmt::new(loc, expr, directive.starts_with(b"i")));
        self.after_rule = false;
        Ok(())
    }

    fn parse_define(&mut self, line: Bytes) -> Result<()> {
        if line.is_empty() {
            error_loc!(Some(&self.loc), "*** empty variable name.");
        }
        self.define_name = Some(line);
        self.num_define_nest = 1;
        self.define_start = 0;
        self.define_start_line = self.loc.line;
        self.after_rule = false;
        Ok(())
    }

    fn parse_inside_define(&mut self, line: Bytes) -> Result<()> {
        let line = line.slice_ref(trim_left_space(&line));
        let directive = Parser::get_directive(&line);
        if directive == b"define" {
            self.num_define_nest += 1;
        } else if directive == b"endef" {
            self.num_define_nest -= 1;
        }
        if self.num_define_nest > 0 {
            if self.define_start == 0 {
                self.define_start = self.l;
            }
            return Ok(());
        }

        let rest = trim_right_space(Parser::remove_comment(trim_left_space(
            &line["endef".len()..],
        )));
        if !rest.is_empty() {
            warn_loc!(Some(&self.loc), "extraneous text after `endef' directive");
        }

        let assign_loc = Loc {
            filename: self.loc.filename,
            line: self.define_start_line,
        };
        let mut mutable_loc = assign_loc.clone();
        let lhs = parse_expr(
            &mut mutable_loc,
            self.define_name.clone().unwrap(),
            ParseExprOpt::Normal,
        )?;
        mutable_loc.line += 1;
        let orig_rhs = if self.define_start > 0 {
            self.buf.slice(self.define_start..(self.l - 1))
        } else {
            Bytes::new()
        };
        let rhs = parse_expr(&mut mutable_loc, orig_rhs.clone(), ParseExprOpt::Define)?;

        self.out_stmts.lock().push(AssignStmt::new(
            assign_loc,
            lhs,
            rhs,
            orig_rhs,
            AssignOp::Eq,
            self.current_directive,
            false,
        ));
        self.define_name = None;
        Ok(())
    }

    fn enter_if(&mut self, stmt: Arc<IfStmt>) {
        self.if_stack.push(IfState {
            stmt: stmt.clone(),
            is_in_else: false,
            num_nest: self.num_if_nest,
        });
        self.out_stmts = stmt.true_stmts.clone();
    }

    fn parse_ifdef(&mut self, line: Bytes, directive: &[u8]) -> Result<()> {
        let loc = self.loc.clone();
        let op = if directive[2] == b'n' {
            CondOp::Ifndef
        } else {
            CondOp::Ifdef
        };
        let mut mutable_loc = loc.clone();
        let lhs = parse_expr(&mut mutable_loc, line, ParseExprOpt::Normal)?;
        let stmt = IfStmt::new(loc, op, lhs, None);
        self.out_stmts.lock().push(stmt.clone());
        self.enter_if(stmt);
        Ok(())
    }

    fn parse_ifeq(&mut self, mut line: Bytes, directive: &[u8]) -> Result<()> {
        let loc = self.loc.clone();
        let op = if directive[2] == b'n' {
            CondOp::Ifneq
        } else {
            CondOp::Ifeq
        };

        if line.is_empty() {
            error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
        }

        let mut mutable_loc = loc.clone();
        let lhs;
        let rhs;
        if line.first() == Some(&b'(') && line.last() == Some(&b')') {
            line = line.slice(1..line.len() - 1);
            let terms = vec![b','];
            let mut n;
            (n, lhs) = parse_expr_impl(
                &mut mutable_loc,
                line.clone(),
                Some(&terms),
                ParseExprOpt::Normal,
                true,
            )?;
            line.advance(n);
            if line.first() != Some(&b',') {
                error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
            }
            line = line.slice_ref(trim_left_space(&line[1..]));
            (n, rhs) = parse_expr_impl_ext(
                &mut mutable_loc,
                line.clone(),
                None,
                ParseExprOpt::Normal,
                false,
                true,
            )?;
            line = line.slice_ref(trim_left_space(&line[n.min(line.len())..]));
        } else {
            if line.is_empty() {
                error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
            }
            let quote = line[0];
            if quote != b'\'' && quote != b'"' {
                error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
            }
            let Some(end) = memchr(quote, &line[1..]) else {
                error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
            };
            lhs = parse_expr(
                &mut mutable_loc,
                line.slice(1..end + 1),
                ParseExprOpt::Normal,
            )?;

            line = line.slice_ref(trim_left_space(&line[end + 2..]));

            if line.is_empty() {
                error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
            }
            let quote = line[0];
            if quote != b'\'' && quote != b'"' {
                error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
            }
            let Some(end) = memchr(quote, &line[1..]) else {
                error_loc!(Some(&self.loc), "*** invalid syntax in conditional.");
            };
            rhs = parse_expr(
                &mut mutable_loc,
                line.slice(1..end + 1),
                ParseExprOpt::Normal,
            )?;
            line = line.slice_ref(trim_left_space(&line[end + 2..]));
        }

        if !line.is_empty() {
            warn_loc!(Some(&self.loc), "extraneous text after `ifeq' directive")
        }

        let stmt = IfStmt::new(loc, op, lhs, Some(rhs));
        self.out_stmts.lock().push(stmt.clone());
        self.enter_if(stmt);
        Ok(())
    }

    fn parse_else(&mut self, line: Bytes) -> Result<()> {
        self.check_if_stack("else")?;
        let st = self.if_stack.last_mut().unwrap();
        if st.is_in_else {
            error_loc!(Some(&self.loc), "*** only one `else' per conditional.");
        }
        st.is_in_else = true;
        self.out_stmts = st.stmt.false_stmts.clone();

        let next_if = trim_left_space(&line);
        if next_if.is_empty() {
            return Ok(());
        }

        self.num_if_nest = st.num_nest + 1;
        if !self.handle_else_if_directive(&line.slice_ref(next_if))? {
            warn_loc!(Some(&self.loc), "extraneous text after `else' directive");
        }
        self.num_if_nest = 0;
        Ok(())
    }

    fn parse_endif(&mut self, line: Bytes) -> Result<()> {
        self.check_if_stack("endif")?;
        if !line.is_empty() {
            error_loc!(Some(&self.loc), "extraneous text after `endif` directive");
        }
        let num_nest = self.if_stack.last().unwrap().num_nest;
        for _ in 0..=num_nest {
            self.if_stack.pop();
        }
        if let Some(st) = self.if_stack.last() {
            if st.is_in_else {
                self.out_stmts = st.stmt.false_stmts.clone();
            } else {
                self.out_stmts = st.stmt.true_stmts.clone();
            }
        } else {
            self.out_stmts = self.stmts.clone();
        }
        Ok(())
    }

    fn is_in_export(&self) -> bool {
        self.current_directive.is_some_and(|d| d.export)
    }

    fn create_export(&mut self, line: &Bytes, is_export: bool) -> Result<()> {
        let loc = self.loc.clone();
        let mut mutable_loc = loc.clone();
        let expr = parse_expr(&mut mutable_loc, line.clone(), ParseExprOpt::Normal)?;
        self.out_stmts
            .lock()
            .push(ExportStmt::new(loc, expr, is_export));
        Ok(())
    }

    fn parse_override(&mut self, line: Bytes) -> Result<()> {
        let mut current_directive = self.current_directive.unwrap_or_default();
        current_directive.is_override = true;
        self.current_directive = Some(current_directive);
        if self.handle_assign_directive(&line)? {
            return Ok(());
        }
        if self.is_in_export() {
            self.create_export(&line, true)?;
        }
        self.parse_rule_or_assign(line)
    }

    fn parse_export(&mut self, line: Bytes) -> Result<()> {
        let mut current_directive = self.current_directive.unwrap_or_default();
        current_directive.export = true;
        self.current_directive = Some(current_directive);
        if self.handle_assign_directive(&line)? {
            return Ok(());
        }
        self.create_export(&line, true)?;
        self.parse_rule_or_assign(line)
    }

    fn parse_unexport(&mut self, line: &Bytes) -> Result<()> {
        self.create_export(line, false)
    }

    fn check_if_stack(&self, keyword: &'static str) -> Result<()> {
        if self.if_stack.is_empty() {
            error_loc!(Some(&self.loc), "*** extraneous `{keyword}'.");
        }
        Ok(())
    }

    fn remove_comment(line: &[u8]) -> &[u8] {
        if let Some(i) = find_outside_paren(line, b"#") {
            return &line[..i];
        }
        line
    }

    fn get_directive(line: &[u8]) -> &[u8] {
        if line.len() < 4 {
            return &[];
        }
        let l = &line[0..9.min(line.len())];
        if let Some(i) = memchr3(b' ', b'\t', b'#', l) {
            return &l[..i];
        }
        l
    }

    fn handle_make_directive(&mut self, line: &Bytes) -> Result<bool> {
        let directive = Parser::get_directive(line);
        let rest = line.slice_ref(trim_right_space(Parser::remove_comment(trim_left_space(
            &line[directive.len()..],
        ))));
        match directive {
            b"include" | b"-include" | b"sinclude" => self.parse_include(rest, directive)?,
            b"define" => self.parse_define(rest)?,
            b"ifdef" | b"ifndef" => self.parse_ifdef(rest, directive)?,
            b"ifeq" | b"ifneq" => self.parse_ifeq(rest, directive)?,
            b"else" => self.parse_else(rest)?,
            b"endif" => self.parse_endif(rest)?,
            b"override" => self.parse_override(rest)?,
            b"export" => self.parse_export(rest)?,
            b"unexport" => self.parse_unexport(&rest)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn handle_else_if_directive(&mut self, line: &Bytes) -> Result<bool> {
        let directive = Parser::get_directive(line);
        let rest = line.slice_ref(trim_right_space(Parser::remove_comment(trim_left_space(
            &line[directive.len()..],
        ))));
        match directive {
            b"ifdef" | b"ifndef" => self.parse_ifdef(rest, directive)?,
            b"ifeq" | b"ifneq" => self.parse_ifeq(rest, directive)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn handle_assign_directive(&mut self, line: &Bytes) -> Result<bool> {
        let directive = Parser::get_directive(line);
        let rest = line.slice_ref(trim_right_space(Parser::remove_comment(trim_left_space(
            &line[directive.len()..],
        ))));
        match directive {
            b"define" => self.parse_define(rest)?,
            b"override" => self.parse_override(rest)?,
            b"export" => self.parse_export(rest)?,
            _ => return Ok(false),
        }
        Ok(true)
    }
}

pub fn parse_file(buf: &Bytes, filename: Symbol) -> Result<Arc<Mutex<Vec<Stmt>>>> {
    collect_stats!("parse file time");
    let loc = Loc { filename, line: 0 };
    parse_buf_no_stats_impl(buf, loc, false)
}

pub fn parse_buf(buf: &Bytes, loc: Loc) -> Result<Arc<Mutex<Vec<Stmt>>>> {
    collect_stats!("parse eval time");
    parse_buf_no_stats_impl(buf, loc, true)
}

pub fn parse_buf_no_stats(buf: &Bytes, loc: Loc) -> Result<Arc<Mutex<Vec<Stmt>>>> {
    parse_buf_no_stats_impl(buf, loc, true)
}

fn parse_buf_no_stats_impl(
    buf: &Bytes,
    loc: Loc,
    fixed_lineno: bool,
) -> Result<Arc<Mutex<Vec<Stmt>>>> {
    let stmts = Arc::new(Mutex::new(Vec::new()));
    let mut p = Parser::with_buf(buf, loc, stmts.clone(), fixed_lineno);
    p.parse()?;
    Ok(stmts)
}

pub struct ParsedAssign<'a> {
    pub lhs: &'a [u8],
    pub rhs: &'a [u8],
    pub op: AssignOp,
}
pub fn parse_assign_statement(line: &[u8], sep: usize) -> ParsedAssign<'_> {
    assert!(sep != 0);
    let mut op = AssignOp::Eq;
    let mut lhs = &line[..sep];
    if lhs.ends_with(b":") {
        lhs = &lhs[..lhs.len() - 1];
        op = AssignOp::ColonEq;
    } else if lhs.ends_with(b"+") {
        lhs = &lhs[..lhs.len() - 1];
        op = AssignOp::PlusEq;
    } else if lhs.ends_with(b"?") {
        lhs = &lhs[..lhs.len() - 1];
        op = AssignOp::QuestionEq;
    }
    lhs = trim_space(lhs);
    let rhs = trim_left_space(&line[line.len().min(sep + 1)..]);
    ParsedAssign { lhs, rhs, op }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_get_directive() {
        assert_eq!(
            Parser::get_directive(&Bytes::from_static(b"ifdef VAR")),
            Bytes::from_static(b"ifdef")
        );
        assert_eq!(
            Parser::get_directive(&Bytes::from_static(b"endif")),
            Bytes::from_static(b"endif")
        );
    }
}
