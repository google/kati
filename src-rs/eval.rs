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

use std::borrow::Cow;
use std::collections::{BTreeSet, HashMap, HashSet};
use std::ffi::{OsStr, OsString};
use std::io::BufWriter;
use std::os::unix::ffi::OsStringExt;
use std::sync::{Arc, LazyLock, Weak};

use anyhow::{Context, Result};
use bytes::{Buf, Bytes};
use memchr::{memchr, memchr2};
use parking_lot::Mutex;

use crate::expr::Evaluable;
use crate::expr::Value;
use crate::flags::FLAGS;
use crate::loc::Loc;
use crate::parser::{parse_assign_statement, parse_buf_no_stats};
use crate::rule::{Rule, is_pattern_rule};
use crate::stmt::{
    AssignOp, AssignStmt, CommandStmt, CondOp, ExportStmt, IfStmt, IncludeStmt, RuleSep, RuleStmt,
    Statement,
};
use crate::strutil::{is_space_byte, trim_leading_curdir, trim_right_space, word_scanner};
use crate::symtab::{ALLOW_RULES_SYM, KATI_READONLY_SYM, MAKEFILE_LIST, SHELL_SYM, Symbol, intern};
use crate::var::{Var, VarOrigin, Variable, Vars};
use crate::{collect_stats_with_slow_report, error_loc, file_cache, log, warn_loc};

pub enum RulesAllowed {
    Allowed,
    Warning,
    Error,
}

/// Whether `export` directives are allowed.
pub enum ExportAllowed {
    /// Export directives are allowed, the default.
    Allowed,
    /// Export directives result in warnings with the specified message.
    Warning(String),
    /// Export directives result in errors with the specified message.
    Error(String),
}

#[derive(Debug, PartialEq, Eq)]
pub enum FrameType {
    Root,       // Root node. Exactly one of this exists.
    Phase,      // Markers for various phases of the execution.
    Parse,      // Initial evaluation pass: include, := variables, etc.
    Call,       // Evaluating the result of a function call
    FunCall,    // Evaluating a function call (not its result)
    Statement,  // Denotes individual statements for better location reporting
    Dependency, // Dependency analysis. += requires variable expansion here.
    Exec,       // Execution phase. Expansion of = and rule-specific variables.
    Ninja,      // Ninja file generation
}

#[derive(Debug)]
pub struct Frame {
    frame_type: FrameType,
    parent: Option<Weak<Frame>>,
    name: Bytes,
    location: Option<Loc>,
    children: Mutex<Vec<Arc<Frame>>>,
}

impl Frame {
    fn new(
        frame_type: FrameType,
        parent: Option<Arc<Frame>>,
        loc: Option<Loc>,
        name: Bytes,
    ) -> Self {
        assert!(parent.is_none() == (frame_type == FrameType::Root));
        Self {
            frame_type,
            parent: parent.map(|p| Arc::downgrade(&p)),
            name,
            location: loc,
            children: Mutex::new(Vec::new()),
        }
    }

    fn add(&self, child: Arc<Frame>) {
        self.children.lock().push(child);
    }

    fn print_json_trace(&self, tf: &mut dyn std::io::Write, indent: usize) -> Result<()> {
        if self.frame_type == FrameType::Root {
            return Ok(());
        }

        let indent_string = " ".repeat(indent);
        let mut desc = String::from_utf8_lossy(&self.name);
        if let Some(loc) = &self.location {
            desc = Cow::Owned(format!("{desc} @ {loc}"));
        }

        let parent = self.parent.clone().unwrap().upgrade();
        let comma = if parent
            .clone()
            .is_some_and(|p| p.frame_type == FrameType::Root)
        {
            ""
        } else {
            ","
        };
        writeln!(tf, "{indent_string}\"{desc}\"{comma}")?;
        if let Some(parent) = parent {
            parent.print_json_trace(tf, indent)?;
        }
        Ok(())
    }
}

pub struct ScopedFrame {
    stack: Arc<Mutex<Vec<Arc<Frame>>>>,
    frame: Option<Arc<Frame>>,
}

impl ScopedFrame {
    fn new(stack: Arc<Mutex<Vec<Arc<Frame>>>>, frame: Option<Arc<Frame>>) -> Self {
        if let Some(frame) = frame.clone() {
            let mut stack = stack.lock();
            stack.last().unwrap().add(frame.clone());
            stack.push(frame);
        }
        Self { stack, frame }
    }
    pub fn current(&self) -> Option<Arc<Frame>> {
        self.frame.clone()
    }
}

impl Drop for ScopedFrame {
    fn drop(&mut self) {
        if let Some(frame) = &self.frame {
            let mut stack = self.stack.lock();
            let last = stack.pop().unwrap();
            assert!(last.name == frame.name);
            assert!(last.location == frame.location);
        }
    }
}

#[derive(Default)]
struct IncludeGraphNode {
    includes: BTreeSet<Bytes>,
}

struct IncludeGraph {
    nodes: HashMap<Bytes, IncludeGraphNode>,
    include_stack: Vec<Arc<Frame>>,
}

impl IncludeGraph {
    fn new() -> Self {
        Self {
            nodes: HashMap::new(),
            include_stack: Vec::new(),
        }
    }

    fn dump_json(&self, tf: &mut dyn std::io::Write) -> Result<()> {
        writeln!(tf, "{{")?;
        write!(tf, "  \"include_graph\": [")?;
        let mut first_node = true;

        for (file, node) in &self.nodes {
            if first_node {
                first_node = false;
                writeln!(tf)?;
            } else {
                writeln!(tf, ",")?;
            }

            writeln!(tf, "    {{")?;
            // TODO(lberki): Quote all these strings properly
            writeln!(tf, "      \"file\": \"{}\",", String::from_utf8_lossy(file))?;
            write!(tf, "      \"includes\": [")?;
            let mut first_include = true;
            for include in &node.includes {
                if first_include {
                    first_include = false;
                    writeln!(tf)?;
                } else {
                    writeln!(tf, ",")?;
                }

                write!(tf, "        \"{}\"", String::from_utf8_lossy(include))?;
            }
            writeln!(tf, "\n      ]")?;
            write!(tf, "    }}")?;
        }
        writeln!(tf, "\n  ]")?;
        writeln!(tf, "}}")?;

        Ok(())
    }

    fn merge_tree_node(&mut self, frame: &Arc<Frame>) {
        if frame.frame_type == FrameType::Parse {
            self.nodes.entry(frame.name.clone()).or_default();

            if let Some(parent_frame) = self.include_stack.last()
                && let Some(parent_node) = self.nodes.get_mut(&parent_frame.name)
            {
                parent_node.includes.insert(frame.name.clone());
            }

            self.include_stack.push(frame.clone());
        }

        for child in &*frame.children.lock() {
            self.merge_tree_node(child);
        }

        if frame.frame_type == FrameType::Parse {
            self.include_stack.pop();
        }
    }
}

static USED_UNDEFINED_VARS: LazyLock<Mutex<HashSet<Symbol>>> =
    LazyLock::new(|| Mutex::new(HashSet::new()));

pub struct Evaluator {
    pub rule_vars: HashMap<Symbol, Arc<Vars>>,
    pub rules: Vec<Rule>,
    pub exports: HashMap<Symbol, bool>,
    symbols_for_eval: HashSet<Symbol>,

    in_rule: bool,
    pub current_scope: Option<Arc<Vars>>,

    pub loc: Option<Loc>,
    is_bootstrap: bool,
    is_commandline: bool,

    trace: bool,
    stack: Arc<Mutex<Vec<Arc<Frame>>>>,
    assignment_tracefile: Option<Box<dyn std::io::Write>>,
    assignment_sep: String,

    pub avoid_io: bool,
    // This value tracks the nest level of make expressions. For
    // example, $(YYY) in $(XXX $(YYY)) is evaluated with depth==2.
    // This will be used to disallow $(shell) in other make constructs.
    pub eval_depth: i32,
    // Commands which should run at ninja-time (i.e., info, warning, and
    // error).
    pub delayed_output_commands: Vec<Bytes>,

    posix_sym: Symbol,
    is_posix: bool,

    /// Whether `export`/`unexport` directives are allowed.
    pub export_allowed: ExportAllowed,

    pub profiled_files: Vec<OsString>,

    pub is_evaluating_command: bool,
}

impl Default for Evaluator {
    fn default() -> Self {
        Self::new()
    }
}

impl Evaluator {
    pub fn new() -> Self {
        Self {
            rule_vars: HashMap::new(),
            rules: Vec::new(),
            exports: HashMap::new(),
            symbols_for_eval: HashSet::new(),

            in_rule: false,
            current_scope: None,

            loc: None,
            is_bootstrap: false,
            is_commandline: false,

            trace: FLAGS.dump_variable_assignment_trace.is_some()
                || FLAGS.dump_include_graph.is_some(),
            stack: Arc::new(Mutex::new(vec![Arc::new(Frame::new(
                FrameType::Root,
                None,
                None,
                Bytes::from_static(b"*root*"),
            ))])),
            assignment_tracefile: None,
            assignment_sep: "\n".to_string(),

            avoid_io: false,
            eval_depth: 0,
            delayed_output_commands: Vec::new(),

            posix_sym: crate::symtab::intern(".POSIX"),
            is_posix: false,

            export_allowed: ExportAllowed::Allowed,

            profiled_files: Vec::new(),

            is_evaluating_command: false,
        }
    }

    pub fn start(&mut self) -> Result<()> {
        let Some(filename) = &FLAGS.dump_variable_assignment_trace else {
            return Ok(());
        };

        if filename == "-" {
            self.assignment_tracefile = Some(Box::new(std::io::stderr()));
        } else {
            let f = std::fs::File::create(filename)?;
            let w = BufWriter::new(f);
            self.assignment_tracefile = Some(Box::new(w));
        }

        let tf = self.assignment_tracefile.as_mut().unwrap();
        writeln!(tf, "{{")?;
        write!(tf, "  \"assignments\": [")?;
        Ok(())
    }

    pub fn finish(&mut self) -> Result<()> {
        if let Some(tf) = self.assignment_tracefile.as_mut() {
            write!(tf, " \n ]\n")?;
            writeln!(tf, "}}")?;
        }
        Ok(())
    }

    pub fn in_bootstrap(&mut self) {
        self.is_bootstrap = true;
        self.is_commandline = false;
    }

    pub fn in_command_line(&mut self) {
        self.is_bootstrap = false;
        self.is_commandline = true;
    }

    pub fn in_toplevel_makefile(&mut self) {
        self.is_bootstrap = false;
        self.is_commandline = false;
    }

    pub fn current_frame(&self) -> Arc<Frame> {
        self.stack.lock().last().unwrap().clone()
    }

    pub fn eval_rhs(
        &mut self,
        lhs: Symbol,
        rhs_v: Arc<Value>,
        orig_rhs: Bytes,
        op: AssignOp,
        is_override: bool,
    ) -> Result<(Var, bool)> {
        let (origin, current_frame) = if self.is_bootstrap {
            (VarOrigin::Default, None)
        } else if self.is_commandline {
            (VarOrigin::CommandLine, None)
        } else if is_override {
            (VarOrigin::Override, self.stack.lock().last().cloned())
        } else {
            (VarOrigin::File, self.stack.lock().last().cloned())
        };

        let result: Var;
        let prev: Option<Var>;
        let mut needs_assign = true;

        match op {
            AssignOp::ColonEq => {
                prev = self.peek_var_in_current_scope(lhs);
                result = Variable::with_simple_value(
                    origin,
                    current_frame,
                    self.loc.clone(),
                    self,
                    &rhs_v,
                )?;
            }
            AssignOp::Eq => {
                prev = self.peek_var_in_current_scope(lhs);
                result = Variable::new_recursive(
                    rhs_v,
                    origin,
                    current_frame,
                    self.loc.clone(),
                    orig_rhs,
                );
            }
            AssignOp::PlusEq => {
                prev = self.lookup_var_in_current_scope(lhs)?;
                if let Some(prev) = prev.clone() {
                    if prev.read().readonly {
                        error_loc!(
                            self.loc.as_ref(),
                            "*** cannot assign to readonly variable: {lhs}"
                        );
                    }
                    result = prev;
                    if result.read().immediate_eval() {
                        let buf = rhs_v.eval_to_buf(self)?;
                        result.write().append_str(&buf, self.current_frame())?;
                    } else {
                        result.write().append_var(
                            rhs_v,
                            self.current_frame(),
                            self.loc.as_ref(),
                        )?;
                    }
                    needs_assign = false;
                } else {
                    result = Variable::new_recursive(
                        rhs_v,
                        origin,
                        current_frame,
                        self.loc.clone(),
                        orig_rhs,
                    );
                }
            }
            AssignOp::QuestionEq => {
                prev = self.lookup_var_in_current_scope(lhs)?;
                if let Some(prev) = prev.clone() {
                    result = prev;
                    needs_assign = false;
                } else {
                    result = Variable::new_recursive(
                        rhs_v,
                        origin,
                        current_frame,
                        self.loc.clone(),
                        orig_rhs,
                    );
                }
            }
        }

        if let Some(prev) = prev {
            let prev = prev.read();
            prev.used(self, &lhs)?;
            if needs_assign && let Some(deprecated) = &prev.deprecated {
                result.write().deprecated = Some(deprecated.clone());
            }
        }

        Ok((result, needs_assign))
    }

    pub fn eval_assign(&mut self, stmt: &AssignStmt) -> Result<()> {
        self.loc = Some(stmt.loc());
        self.in_rule = false;
        let lhs = stmt.get_lhs_symbol(self)?;

        if lhs == *KATI_READONLY_SYM {
            let rhs = stmt.rhs.eval_to_buf(self)?;
            for name in word_scanner(&rhs) {
                let name = intern(rhs.slice_ref(name));
                let Some(var) = name.get_global_var() else {
                    error_loc!(self.loc.as_ref(), "*** unknown variable: {name}");
                };
                var.write().readonly = true;
            }
            return Ok(());
        }

        let is_override = stmt.directive.map(|v| v.is_override).unwrap_or(false);
        let (var, needs_assign) = self.eval_rhs(
            lhs,
            stmt.rhs.clone(),
            stmt.orig_rhs.clone(),
            stmt.op,
            is_override,
        )?;
        if needs_assign {
            let mut readonly = false;
            lhs.set_global_var(var.clone(), is_override, Some(&mut readonly))?;
            if readonly {
                error_loc!(
                    self.loc.as_ref(),
                    "*** cannot assign to readonly variable: {lhs}"
                );
            }
        }

        if stmt.is_final {
            var.write().readonly = true
        }
        self.trace_variable_assign(&lhs, &var)?;
        Ok(())
    }

    // With rule broken into
    //   <before_term> <term> <after_term>
    // parses <before_term> into Symbol instances until encountering ':'
    // Returns the remainder of <before_term>.
    pub fn parse_rule_targets(
        loc: &Loc,
        before_term: &Bytes,
    ) -> Result<(Bytes, Vec<Symbol>, bool)> {
        let Some(idx) = memchr(b':', before_term) else {
            error_loc!(Some(loc), "*** missing separator.");
        };
        let targets_string = before_term.slice(0..idx);
        let after = before_term.slice(idx + 1..);
        let mut pattern_rule_count = 0;
        let mut targets: Vec<Symbol> = Vec::new();
        for word in word_scanner(&targets_string) {
            let target = targets_string.slice_ref(trim_leading_curdir(word));
            targets.push(intern(target.clone()));
            if is_pattern_rule(&target) {
                pattern_rule_count += 1;
            }
        }
        // Check consistency: either all outputs are patterns or none.
        if pattern_rule_count > 0 && pattern_rule_count != targets.len() {
            error_loc!(
                Some(loc),
                "*** mixed implicit and normal rules: deprecated syntax"
            );
        }
        Ok((after, targets, pattern_rule_count > 0))
    }

    // Strip leading spaces and trailing spaces and colons.
    pub fn format_rule_error(before_term: &[u8]) -> String {
        let before_term = String::from_utf8_lossy(before_term).into_owned();
        if before_term.is_empty() {
            return before_term;
        }
        before_term
            .trim_ascii_start()
            .trim_end_matches(|c: char| c.is_ascii_whitespace() || c == ':')
            .to_string()
    }

    pub fn mark_vars_readonly(&mut self, vars_list: &Value) -> Result<()> {
        let vars_list_string = vars_list.eval_to_buf(self)?;
        for name in word_scanner(&vars_list_string) {
            let name = intern(vars_list_string.slice_ref(name));
            let Some(var) = self.current_scope.as_ref().unwrap().lookup(name) else {
                error_loc!(self.loc.as_ref(), "*** unknown variable: {name}");
            };
            var.write().readonly = true;
        }
        Ok(())
    }

    pub fn eval_rule_specific_assign(
        &mut self,
        targets: &[Symbol],
        stmt: &RuleStmt,
        after_targets: &Bytes,
        separator_pos: usize,
    ) -> Result<()> {
        let assign = parse_assign_statement(after_targets, separator_pos);
        let var_sym = intern(after_targets.slice_ref(assign.lhs));
        let is_final = stmt.sep == RuleSep::FinalEq;
        for target in targets {
            let scope = self
                .rule_vars
                .entry(*target)
                .or_insert_with(|| Arc::new(Vars::new()))
                .clone();

            let rhs = if assign.rhs.is_empty() {
                stmt.rhs.clone()
            } else if let Some(stmt_rhs) = stmt.rhs.clone() {
                let sep = if stmt.sep == RuleSep::Semicolon {
                    b" ; "
                } else {
                    b" = "
                };
                Some(Arc::new(Value::List(
                    self.loc.clone(),
                    vec![
                        Arc::new(Value::Literal(None, after_targets.slice_ref(assign.rhs))),
                        Arc::new(Value::Literal(None, Bytes::from_static(sep))),
                        stmt_rhs,
                    ],
                )))
            } else {
                Some(Arc::new(Value::Literal(
                    None,
                    after_targets.slice_ref(assign.rhs),
                )))
            };

            self.current_scope = Some(scope);
            if var_sym == *KATI_READONLY_SYM {
                if let Some(rhs) = rhs {
                    self.mark_vars_readonly(&rhs)?;
                }
            } else {
                let (rhs_var, needs_assign) = self.eval_rhs(
                    var_sym,
                    rhs.unwrap(),
                    Bytes::from_static(b"*TODO*"),
                    assign.op,
                    false,
                )?;
                if needs_assign {
                    let mut readonly = false;
                    rhs_var.write().assign_op = Some(assign.op);
                    self.current_scope.as_ref().unwrap().assign(
                        var_sym,
                        rhs_var.clone(),
                        &mut readonly,
                    )?;
                    if readonly {
                        error_loc!(
                            self.loc.as_ref(),
                            "*** cannot assign to readonly variable: {var_sym}"
                        );
                    }
                }
                if is_final {
                    rhs_var.write().readonly = true;
                }
            }
            self.current_scope = None
        }
        Ok(())
    }

    pub fn eval_rule(&mut self, stmt: &RuleStmt) -> Result<()> {
        self.loc = Some(stmt.loc());
        self.in_rule = false;

        let before_term = stmt.lhs.eval_to_buf(self)?;
        // See semicolon.mk.
        if before_term.iter().all(|c| b" \t\n;".contains(c)) {
            if stmt.sep == RuleSep::Semicolon {
                error_loc!(self.loc.as_ref(), "*** missing rule before commands.");
            }
            return Ok(());
        }

        let (mut after_targets, targets, is_pattern_rule) =
            Evaluator::parse_rule_targets(self.loc.as_ref().unwrap(), &before_term)?;
        let is_double_colon = after_targets.starts_with(b":");
        if is_double_colon {
            after_targets.advance(1);
        }

        // Figure out if this is a rule-specific variable assignment.
        // It is an assignment when either after_targets contains an assignment token
        // or separator is an assignment token, but only if there is no ';' before the
        // first assignment token.
        let mut separator_pos = memchr2(b'=', b';', &after_targets);
        let separator = if let Some(separator_pos) = separator_pos {
            Some(after_targets[separator_pos])
        } else if stmt.sep == RuleSep::Eq || stmt.sep == RuleSep::FinalEq {
            separator_pos = Some(after_targets.len());
            Some(b'=')
        } else {
            None
        };

        // If variable name is not empty, we have rule- or target-specific
        // variable assignment.
        if separator == Some(b'=')
            && let Some(separator_pos) = separator_pos
            && separator_pos > 0
        {
            return self.eval_rule_specific_assign(&targets, stmt, &after_targets, separator_pos);
        }

        if separator_pos == Some(0) {
            // We used to make this a warning and otherwise accept it, but Make 4.1
            // calls this out as an error, so let's follow.
            error_loc!(self.loc.as_ref(), "*** empty variable name.");
        }

        let mut rule = Rule::new(self.loc.clone().unwrap(), is_double_colon);
        if is_pattern_rule {
            rule.output_patterns = targets;
        } else {
            rule.outputs = targets;
        }
        rule.parse_prerequisites(&after_targets, separator_pos, stmt)?;

        if stmt.sep == RuleSep::Semicolon {
            rule.cmds.push(stmt.rhs.clone().unwrap());
        }

        for o in &rule.outputs {
            if o == &self.posix_sym {
                self.is_posix = true;
            }
        }

        log!("Rule: {:?}", rule);
        match self.get_allow_rules()? {
            RulesAllowed::Warning => {
                warn_loc!(
                    self.loc.as_ref(),
                    "warning: Rule not allowed here for target: {}",
                    Evaluator::format_rule_error(&before_term)
                );
            }
            RulesAllowed::Error => {
                error_loc!(
                    self.loc.as_ref(),
                    "*** Rule not allowed here for target: {}",
                    Evaluator::format_rule_error(&before_term),
                );
            }
            RulesAllowed::Allowed => {}
        }
        self.rules.push(rule);
        self.in_rule = true;
        Ok(())
    }

    pub fn eval_command(&mut self, stmt: &CommandStmt) -> Result<()> {
        self.loc = Some(stmt.loc());

        if !self.in_rule {
            let stmts = parse_buf_no_stats(&stmt.orig(), stmt.loc())?;
            let stmts = stmts.lock();
            for a in &*stmts {
                a.eval(self)?;
            }
            return Ok(());
        }

        let last_rule = self.rules.last_mut().unwrap();
        last_rule.cmds.push(stmt.expr.clone());
        if last_rule.cmd_loc.is_none() {
            last_rule.cmd_loc = Some(stmt.loc());
        }
        log!("Command: {:?}", stmt.expr);

        Ok(())
    }

    pub fn eval_if(&mut self, stmt: &IfStmt) -> Result<()> {
        self.loc = Some(stmt.loc());

        let is_true = match stmt.op {
            CondOp::Ifdef | CondOp::Ifndef => {
                let var_name = stmt.lhs.eval_to_buf(self)?;
                let lhs = trim_right_space(&var_name);
                if lhs.iter().any(is_space_byte) {
                    error_loc!(self.loc.as_ref(), "*** invalid syntax in conditional.");
                }
                let lhs = intern(var_name.slice_ref(lhs));
                if let Some(v) = self.lookup_var_in_current_scope(lhs)? {
                    let v = v.read();
                    v.used(self, &lhs)?;
                    v.string()?.is_empty() == (stmt.op == CondOp::Ifndef)
                } else {
                    stmt.op == CondOp::Ifndef
                }
            }
            CondOp::Ifeq | CondOp::Ifneq => {
                let lhs = stmt.lhs.eval_to_buf(self)?;
                let rhs = stmt
                    .rhs
                    .as_ref()
                    .map(|v| v.eval_to_buf(self))
                    .unwrap_or_else(|| Ok(Bytes::new()))?;
                (lhs == rhs) == (stmt.op == CondOp::Ifeq)
            }
        };

        let stmts = match is_true {
            true => &stmt.true_stmts,
            false => &stmt.false_stmts,
        };
        let stmts = stmts.lock();
        for a in stmts.iter() {
            log!("{:?}", a);
            a.eval(self)?;
        }
        Ok(())
    }

    pub fn do_include(&mut self, fname: &Bytes) -> Result<()> {
        let filename = OsString::from_vec(fname.to_vec());
        collect_stats_with_slow_report!("included makefiles", &filename);

        let Some(mk) = file_cache::get_makefile(&filename)? else {
            error_loc!(
                self.loc.as_ref(),
                "{} does not exist",
                filename.to_string_lossy()
            );
        };

        let v = fname.slice_ref(trim_leading_curdir(fname));
        if let Some(var_list) = self.lookup_var(*MAKEFILE_LIST)? {
            var_list.write().append_str(&v, self.current_frame())?;
        } else {
            MAKEFILE_LIST.set_global_var(
                Variable::with_simple_string(
                    v,
                    VarOrigin::File,
                    Some(self.current_frame()),
                    self.loc.clone(),
                ),
                false,
                None,
            )?;
        }
        for stmt in mk.stmts.lock().iter() {
            log!("{stmt:?}");
            stmt.eval(self)?;
        }

        if !self.profiled_files.is_empty() {
            for mk in std::mem::take(&mut self.profiled_files) {
                STATS.mark_interesting(mk);
            }
        }
        Ok(())
    }

    pub fn eval_include(&mut self, stmt: &IncludeStmt) -> Result<()> {
        self.loc = Some(stmt.loc());
        self.in_rule = false;

        let pats = stmt.expr.eval_to_buf(self)?;
        for pat in word_scanner(&pats) {
            let pat = pats.slice_ref(pat);
            let files = crate::fileutil::glob(pat.clone());

            if stmt.should_exist {
                match files.as_ref() {
                    Err(err) => {
                        // TODO: Kati does not support building a missing include file.
                        error_loc!(
                            self.loc.as_ref(),
                            "{}: {err}",
                            String::from_utf8_lossy(&pat)
                        );
                    }
                    Ok(files) => {
                        if files.is_empty() {
                            error_loc!(
                                self.loc.as_ref(),
                                "{}: Not found",
                                String::from_utf8_lossy(&pat)
                            );
                        }
                    }
                }
            }
            let Ok(files) = files.as_ref() else {
                continue;
            };

            for fname in files {
                if !stmt.should_exist
                    && FLAGS
                        .ignore_optional_include_pattern
                        .as_ref()
                        .map(|p| p.matches(fname))
                        .unwrap_or(false)
                {
                    continue;
                }

                {
                    let _frame = self.enter(FrameType::Parse, fname.clone(), stmt.loc());
                    self.do_include(fname)
                        .with_context(|| format!("In file included from {}:", stmt.loc()))?;
                }
            }
        }

        Ok(())
    }

    pub fn eval_export(&mut self, stmt: &ExportStmt) -> Result<()> {
        self.loc = Some(stmt.loc());
        self.in_rule = false;

        let exports = stmt.expr.eval_to_buf(self)?;
        for tok in word_scanner(&exports) {
            let equal_index = memchr(b'=', tok);
            let lhs;
            if equal_index == Some(0)
                || (equal_index == Some(1)
                    && (tok.starts_with(b":") || tok.starts_with(b"?") || tok.starts_with(b"+")))
            {
                // Do not export tokens after an assignment.
                break;
            } else if let Some(equal_index) = equal_index {
                let assign = parse_assign_statement(tok, equal_index);
                lhs = assign.lhs;
            } else {
                lhs = tok;
            }
            let sym = intern(exports.slice_ref(lhs));
            self.exports.insert(sym, stmt.is_export);

            let prefix = if stmt.is_export { "" } else { "un" };
            match &self.export_allowed {
                ExportAllowed::Allowed => {}
                ExportAllowed::Error(msg) => error_loc!(
                    self.loc.as_ref(),
                    "*** {sym}: {prefix}export is obsolete{msg}."
                ),
                ExportAllowed::Warning(msg) => warn_loc!(
                    self.loc.as_ref(),
                    "{sym}: {prefix}export has been deprecated{msg}."
                ),
            }
        }
        Ok(())
    }

    pub fn lookup_var_global(&self, name: Symbol) -> Option<Var> {
        let v = name.get_global_var();
        if v.is_none() {
            USED_UNDEFINED_VARS.lock().insert(name);
        }
        v
    }

    pub fn is_traced(&self, name: &Symbol) -> bool {
        if self.assignment_tracefile.is_none() {
            return false;
        }

        // trace every variable unless filtered
        if FLAGS.traced_variables_pattern.is_empty() {
            return true;
        }

        let name = name.as_bytes();
        for pat in FLAGS.traced_variables_pattern.iter() {
            if pat.matches(&name) {
                return true;
            }
        }
        false
    }

    pub fn trace_variable_lookup(
        &mut self,
        operation: &'static str,
        name: &Symbol,
        var: &Option<Var>,
    ) -> Result<()> {
        if !self.is_traced(name) {
            return Ok(());
        }
        let current_frame = self.current_frame();
        let Some(tf) = self.assignment_tracefile.as_mut() else {
            return Ok(());
        };
        write!(tf, "{}", self.assignment_sep)?;
        self.assignment_sep = ",\n".to_string();
        writeln!(tf, "    {{")?;
        writeln!(tf, "      \"name\": \"{name}\",")?;
        writeln!(tf, "      \"operation\": \"{operation}\",")?;
        writeln!(tf, "      \"defined\": {},", var.is_some())?;
        writeln!(tf, "      \"reference_stack\": [")?;
        current_frame.print_json_trace(tf, 8)?;
        writeln!(tf, "      ]")?;
        write!(tf, "    }}")?;
        Ok(())
    }

    pub fn trace_variable_assign(&mut self, name: &Symbol, var: &Var) -> Result<()> {
        if !self.is_traced(name) {
            return Ok(());
        }
        let Some(tf) = self.assignment_tracefile.as_mut() else {
            return Ok(());
        };
        write!(tf, "{}", self.assignment_sep)?;
        self.assignment_sep = ",\n".to_string();
        writeln!(tf, "    {{")?;
        writeln!(tf, "      \"name\": \"{name}\",")?;
        writeln!(tf, "      \"operation\": \"assign\",")?;
        write!(tf, "      \"value\": \"{var:?}\"")?;
        if let Some(definition) = var.read().definition().clone() {
            writeln!(tf, ",\n")?;
            writeln!(tf, "      \"value_stack\": [")?;
            definition.print_json_trace(tf, 8)?;
            writeln!(tf, "      ]")?;
        }
        write!(tf, "    }}")?;
        Ok(())
    }

    pub fn lookup_var_for_eval(&mut self, name: Symbol) -> Result<Option<Var>> {
        if let Some(var) = self.lookup_var(name)? {
            if self.symbols_for_eval.contains(&name) {
                error_loc!(
                    var.read().loc().as_ref(),
                    "*** Recursive variable \"{name}\" references itself (eventually)."
                );
            }
            self.symbols_for_eval.insert(name);
            return Ok(Some(var));
        }
        Ok(None)
    }

    pub fn var_eval_complete(&mut self, name: Symbol) {
        self.symbols_for_eval.remove(&name);
    }

    pub fn lookup_var(&mut self, name: Symbol) -> Result<Option<Var>> {
        let mut result = None;

        if let Some(current_scope) = &self.current_scope {
            result = current_scope.lookup(name);
        }

        if result.is_none() {
            result = self.lookup_var_global(name);
        }

        self.trace_variable_lookup("lookup", &name, &result)?;
        Ok(result)
    }

    pub fn peek_var(&self, name: Symbol) -> Option<Var> {
        let mut result = None;

        if let Some(current_scope) = &self.current_scope {
            result = current_scope.peek(name);
        }

        if result.is_none() {
            result = name.peek_global_var();
        }

        result
    }

    pub fn lookup_var_in_current_scope(&mut self, name: Symbol) -> Result<Option<Var>> {
        let result = if let Some(current_scope) = &self.current_scope {
            current_scope.lookup(name)
        } else {
            self.lookup_var_global(name)
        };

        self.trace_variable_lookup("scope lookup", &name, &result)?;
        Ok(result)
    }

    pub fn peek_var_in_current_scope(&self, name: Symbol) -> Option<Var> {
        if let Some(current_scope) = &self.current_scope {
            current_scope.peek(name)
        } else {
            name.peek_global_var()
        }
    }

    pub fn eval_var(&mut self, name: Symbol) -> Result<Bytes> {
        if let Some(var) = self.lookup_var(name)? {
            var.read().eval_to_buf(self)
        } else {
            Ok(Bytes::new())
        }
    }

    pub fn enter(&mut self, frame_type: FrameType, name: Bytes, loc: Loc) -> ScopedFrame {
        if !self.trace {
            return ScopedFrame::new(self.stack.clone(), None);
        }

        let parent = self.stack.lock().last().cloned();
        let frame = Frame::new(frame_type, parent, Some(loc), name);
        ScopedFrame::new(self.stack.clone(), Some(Arc::new(frame)))
    }

    pub fn get_shell(&mut self) -> Result<Bytes> {
        self.eval_var(*SHELL_SYM)
    }

    pub fn get_shell_flag(&self) -> &'static [u8] {
        if self.is_posix { b"-ec" } else { b"-c" }
    }

    fn get_allow_rules(&mut self) -> Result<RulesAllowed> {
        Ok(match self.eval_var(*ALLOW_RULES_SYM)?.as_ref() {
            b"warning" => RulesAllowed::Warning,
            b"error" => RulesAllowed::Error,
            _ => RulesAllowed::Allowed,
        })
    }

    pub fn dump_include_json(&self, filename: &OsStr) -> Result<()> {
        let mut graph = IncludeGraph::new();
        graph.merge_tree_node(self.stack.lock().first().unwrap());
        let mut w: Box<dyn std::io::Write> = if filename == OsStr::new("-") {
            Box::new(std::io::stdout())
        } else {
            let f = std::fs::File::create(filename)?;
            Box::new(BufWriter::new(f))
        };

        graph.dump_json(&mut w)?;
        Ok(())
    }

    pub fn used_undefined_vars() -> HashSet<Symbol> {
        USED_UNDEFINED_VARS.lock().clone()
    }
}
