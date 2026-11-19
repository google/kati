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
use bytes::Bytes;
use parking_lot::Mutex;
use std::{fmt::Debug, sync::Arc};

use crate::{
    error_loc,
    eval::Evaluator,
    expr::{Evaluable, Value},
    loc::Loc,
    strutil::no_line_break,
    symtab::{Symbol, intern},
};

pub type Stmt = Statement;

pub enum Statement {
    Rule(RuleStmt),
    Assign(AssignStmt),
    Command(CommandStmt),
    If(IfStmt),
    Include(IncludeStmt),
    Export(ExportStmt),
}

impl Statement {
    pub fn loc(&self) -> Loc {
        match self {
            Statement::Rule(s) => s.loc(),
            Statement::Assign(s) => s.loc(),
            Statement::Command(s) => s.loc(),
            Statement::If(s) => s.loc(),
            Statement::Include(s) => s.loc(),
            Statement::Export(s) => s.loc(),
        }
    }

    pub fn eval(&self, ev: &mut Evaluator) -> Result<()> {
        match self {
            Statement::Rule(s) => ev.eval_rule(s),
            Statement::Assign(s) => ev.eval_assign(s),
            Statement::Command(s) => ev.eval_command(s),
            Statement::If(s) => ev.eval_if(s),
            Statement::Include(s) => ev.eval_include(s),
            Statement::Export(s) => ev.eval_export(s),
        }
    }
}

impl Debug for Statement {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Statement::Rule(s) => s.fmt(f),
            Statement::Assign(s) => s.fmt(f),
            Statement::Command(s) => s.fmt(f),
            Statement::If(s) => s.fmt(f),
            Statement::Include(s) => s.fmt(f),
            Statement::Export(s) => s.fmt(f),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AssignOp {
    Eq,
    ColonEq,
    PlusEq,
    QuestionEq,
}

#[derive(Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct AssignDirective {
    pub is_override: bool,
    pub export: bool,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CondOp {
    Ifeq,
    Ifneq,
    Ifdef,
    Ifndef,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RuleSep {
    Null,
    Semicolon,
    Eq,
    FinalEq,
}

pub struct RuleStmt {
    loc: Loc,

    pub lhs: Arc<Value>,
    pub sep: RuleSep,
    pub rhs: Option<Arc<Value>>,
}

impl Debug for RuleStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "RuleStmt(lhs={:?} sep={:?} rhs={:?} loc={})",
            self.lhs, self.sep, self.rhs, self.loc
        )
    }
}

impl RuleStmt {
    pub fn new_stmt(loc: Loc, lhs: Arc<Value>, sep: RuleSep, rhs: Option<Arc<Value>>) -> Stmt {
        Statement::Rule(RuleStmt { loc, lhs, sep, rhs })
    }

    pub fn loc(&self) -> Loc {
        self.loc
    }
}

pub struct AssignStmt {
    loc: Loc,

    pub lhs: Arc<Value>,
    pub rhs: Arc<Value>,
    pub orig_rhs: Bytes,
    pub op: AssignOp,
    pub directive: Option<AssignDirective>,
    pub is_final: bool,

    lhs_sym_cache: Mutex<Option<Symbol>>,
}

impl Debug for AssignStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "AssignStmt(lhs={:?} rhs={:?} ({}) opstr={:?} dir={:?} loc={})",
            self.lhs,
            self.rhs,
            no_line_break(String::from_utf8_lossy(&self.orig_rhs)),
            self.op,
            self.directive,
            self.loc
        )
    }
}

impl AssignStmt {
    pub fn new_stmt(
        loc: Loc,
        lhs: Arc<Value>,
        rhs: Arc<Value>,
        orig_rhs: Bytes,
        op: AssignOp,
        directive: Option<AssignDirective>,
        is_final: bool,
    ) -> Stmt {
        Statement::Assign(AssignStmt {
            loc,
            lhs,
            rhs,
            orig_rhs,
            op,
            directive,
            is_final,
            lhs_sym_cache: Mutex::new(None),
        })
    }

    pub fn loc(&self) -> Loc {
        self.loc
    }

    pub fn get_lhs_symbol(&self, ev: &mut Evaluator) -> Result<Symbol> {
        if let Value::Literal(_, v) = &*self.lhs {
            if v.is_empty() {
                error_loc!(Some(&self.loc), "*** empty variable name.");
            }

            let mut cache = self.lhs_sym_cache.lock();
            if cache.is_none() {
                *cache = Some(intern(v.clone()));
            }
            return Ok((*cache).unwrap());
        }

        let buf = self.lhs.eval_to_buf(ev)?;
        if buf.is_empty() {
            error_loc!(Some(&self.loc), "*** empty variable name.");
        }
        Ok(intern(buf))
    }
}

pub struct CommandStmt {
    loc: Loc,
    orig: Bytes,

    pub expr: Arc<Value>,
}

impl Debug for CommandStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "CommandStmt({:?}, loc={})", self.expr, self.loc)
    }
}

impl CommandStmt {
    pub fn new_stmt(loc: Loc, orig: Bytes, expr: Arc<Value>) -> Stmt {
        Statement::Command(CommandStmt { loc, orig, expr })
    }

    pub fn loc(&self) -> Loc {
        self.loc
    }

    pub fn orig(&self) -> &Bytes {
        &self.orig
    }
}

pub struct IfStmt {
    loc: Loc,

    pub op: CondOp,
    pub lhs: Arc<Value>,
    pub rhs: Option<Arc<Value>>,
    pub true_stmts: Vec<Stmt>,
    pub false_stmts: Vec<Stmt>,
}

impl Debug for IfStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "IfStmt(op={:?}, lhs={:?}, rhs={:?} t={} f={} loc={}",
            self.op,
            self.lhs,
            self.rhs,
            self.true_stmts.len(),
            self.false_stmts.len(),
            self.loc
        )
    }
}

impl IfStmt {
    pub fn new(loc: Loc, op: CondOp, lhs: Arc<Value>, rhs: Option<Arc<Value>>) -> IfStmt {
        IfStmt {
            loc,
            op,
            lhs,
            rhs,
            true_stmts: Vec::new(),
            false_stmts: Vec::new(),
        }
    }

    pub fn loc(&self) -> Loc {
        self.loc
    }
}

pub struct IncludeStmt {
    loc: Loc,

    pub expr: Arc<Value>,
    pub should_exist: bool,
}

impl Debug for IncludeStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "IncludeStmt({:?}, loc={})", self.expr, self.loc)
    }
}

impl IncludeStmt {
    pub fn new_stmt(loc: Loc, expr: Arc<Value>, should_exist: bool) -> Stmt {
        Statement::Include(IncludeStmt {
            loc,
            expr,
            should_exist,
        })
    }

    pub fn loc(&self) -> Loc {
        self.loc
    }
}

pub struct ExportStmt {
    loc: Loc,

    pub expr: Arc<Value>,
    pub is_export: bool,
}

impl Debug for ExportStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "ExportStmt({:?}, {}, loc={})",
            self.expr, self.is_export, self.loc
        )
    }
}

impl ExportStmt {
    pub fn new_stmt(loc: Loc, expr: Arc<Value>, is_export: bool) -> Stmt {
        Statement::Export(ExportStmt {
            loc,
            expr,
            is_export,
        })
    }

    pub fn loc(&self) -> Loc {
        self.loc
    }
}
