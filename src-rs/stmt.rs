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

pub type Stmt = Arc<dyn Statement + Send + Sync>;

pub trait Statement: Debug {
    fn loc(&self) -> Loc;
    fn orig(&self) -> Bytes;

    fn eval(&self, ev: &mut Evaluator) -> Result<()>;
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
    orig: Bytes,

    pub lhs: Arc<Value>,
    pub sep: RuleSep,
    pub rhs: Option<Arc<Value>>,
}

impl Statement for RuleStmt {
    fn loc(&self) -> Loc {
        self.loc.clone()
    }

    fn orig(&self) -> Bytes {
        self.orig.clone()
    }

    fn eval(&self, ev: &mut Evaluator) -> Result<()> {
        ev.eval_rule(self)
    }
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
    pub fn new(loc: Loc, lhs: Arc<Value>, sep: RuleSep, rhs: Option<Arc<Value>>) -> Arc<RuleStmt> {
        Arc::new(RuleStmt {
            loc,
            orig: Bytes::new(),
            lhs,
            sep,
            rhs,
        })
    }
}

pub struct AssignStmt {
    loc: Loc,
    orig: Bytes,

    pub lhs: Arc<Value>,
    pub rhs: Arc<Value>,
    pub orig_rhs: Bytes,
    pub op: AssignOp,
    pub directive: Option<AssignDirective>,
    pub is_final: bool,

    lhs_sym_cache: Mutex<Option<Symbol>>,
}

impl Statement for AssignStmt {
    fn loc(&self) -> Loc {
        self.loc.clone()
    }

    fn orig(&self) -> Bytes {
        self.orig.clone()
    }

    fn eval(&self, ev: &mut Evaluator) -> Result<()> {
        ev.eval_assign(self)
    }
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
    pub fn new(
        loc: Loc,
        lhs: Arc<Value>,
        rhs: Arc<Value>,
        orig_rhs: Bytes,
        op: AssignOp,
        directive: Option<AssignDirective>,
        is_final: bool,
    ) -> Arc<AssignStmt> {
        Arc::new(AssignStmt {
            loc,
            orig: Bytes::new(),
            lhs,
            rhs,
            orig_rhs,
            op,
            directive,
            is_final,
            lhs_sym_cache: Mutex::new(None),
        })
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

impl Statement for CommandStmt {
    fn loc(&self) -> Loc {
        self.loc.clone()
    }
    fn orig(&self) -> Bytes {
        self.orig.clone()
    }
    fn eval(&self, ev: &mut Evaluator) -> Result<()> {
        ev.eval_command(self)
    }
}

impl Debug for CommandStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "CommandStmt({:?}, loc={})", self.expr, self.loc)
    }
}

impl CommandStmt {
    pub fn new(loc: Loc, orig: Bytes, expr: Arc<Value>) -> Arc<CommandStmt> {
        Arc::new(CommandStmt { loc, orig, expr })
    }
}

pub struct IfStmt {
    loc: Loc,
    orig: Bytes,

    pub op: CondOp,
    pub lhs: Arc<Value>,
    pub rhs: Option<Arc<Value>>,
    pub true_stmts: Arc<Mutex<Vec<Stmt>>>,
    pub false_stmts: Arc<Mutex<Vec<Stmt>>>,
}

impl Statement for IfStmt {
    fn loc(&self) -> Loc {
        self.loc.clone()
    }
    fn orig(&self) -> Bytes {
        self.orig.clone()
    }
    fn eval(&self, ev: &mut Evaluator) -> Result<()> {
        ev.eval_if(self)
    }
}

impl Debug for IfStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "IfStmt(op={:?}, lhs={:?}, rhs={:?} t={} f={} loc={}",
            self.op,
            self.lhs,
            self.rhs,
            self.true_stmts.lock().len(),
            self.false_stmts.lock().len(),
            self.loc
        )
    }
}

impl IfStmt {
    pub fn new(loc: Loc, op: CondOp, lhs: Arc<Value>, rhs: Option<Arc<Value>>) -> Arc<IfStmt> {
        Arc::new(IfStmt {
            loc,
            orig: Bytes::new(),
            op,
            lhs,
            rhs,
            true_stmts: Arc::new(Mutex::new(Vec::new())),
            false_stmts: Arc::new(Mutex::new(Vec::new())),
        })
    }
}

pub struct IncludeStmt {
    loc: Loc,
    orig: Bytes,

    pub expr: Arc<Value>,
    pub should_exist: bool,
}

impl Statement for IncludeStmt {
    fn loc(&self) -> Loc {
        self.loc.clone()
    }
    fn orig(&self) -> Bytes {
        self.orig.clone()
    }
    fn eval(&self, ev: &mut Evaluator) -> Result<()> {
        ev.eval_include(self)
    }
}

impl Debug for IncludeStmt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "IncludeStmt({:?}, loc={})", self.expr, self.loc)
    }
}

impl IncludeStmt {
    pub fn new(loc: Loc, expr: Arc<Value>, should_exist: bool) -> Arc<IncludeStmt> {
        Arc::new(IncludeStmt {
            loc,
            orig: Bytes::new(),
            expr,
            should_exist,
        })
    }
}

pub struct ExportStmt {
    loc: Loc,
    orig: Bytes,

    pub expr: Arc<Value>,
    pub is_export: bool,
}

impl Statement for ExportStmt {
    fn loc(&self) -> Loc {
        self.loc.clone()
    }
    fn orig(&self) -> Bytes {
        self.orig.clone()
    }
    fn eval(&self, ev: &mut Evaluator) -> Result<()> {
        ev.eval_export(self)
    }
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
    pub fn new(loc: Loc, expr: Arc<Value>, is_export: bool) -> Arc<ExportStmt> {
        Arc::new(ExportStmt {
            loc,
            orig: Bytes::new(),
            expr,
            is_export,
        })
    }
}
