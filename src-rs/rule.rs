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

use std::fmt::Debug;
use std::sync::Arc;

use anyhow::Result;
use bytes::Bytes;
use memchr::memchr;

use crate::expr::Value;
use crate::loc::Loc;
use crate::stmt::{RuleSep, RuleStmt};
use crate::strutil::{Pattern, trim_leading_curdir, word_scanner};
use crate::symtab::{Symbol, intern};
use crate::{error_loc, warn_loc};

#[derive(Clone)]
pub struct Rule {
    pub outputs: Vec<Symbol>,
    pub inputs: Vec<Symbol>,
    pub order_only_inputs: Vec<Symbol>,
    pub output_patterns: Vec<Symbol>,
    pub validations: Vec<Symbol>,
    pub is_double_colon: bool,
    pub is_suffix_rule: bool,
    pub cmds: Vec<Arc<Value>>,
    pub loc: Loc,
    pub cmd_loc: Option<Loc>,
}

impl Rule {
    pub fn new(loc: Loc, is_double_colon: bool) -> Self {
        Self {
            outputs: Vec::new(),
            inputs: Vec::new(),
            order_only_inputs: Vec::new(),
            output_patterns: Vec::new(),
            validations: Vec::new(),
            is_double_colon,
            is_suffix_rule: false,
            cmds: Vec::new(),
            loc,
            cmd_loc: None,
        }
    }

    fn parse_inputs(&mut self, inputs_str: &Bytes) {
        let mut is_order_only = false;
        for input in word_scanner(inputs_str) {
            if input == b"|" {
                is_order_only = true;
                continue;
            }
            let input_sym = intern(inputs_str.slice_ref(trim_leading_curdir(input)));
            if is_order_only {
                self.order_only_inputs.push(input_sym);
            } else {
                self.inputs.push(input_sym);
            }
        }
    }

    pub fn parse_prerequisites(
        &mut self,
        line: &Bytes,
        separator_pos: Option<usize>,
        rule_stmt: &RuleStmt,
    ) -> Result<()> {
        // line is either
        //    prerequisites [ ; command ]
        // or
        //    target-prerequisites : prereq-patterns [ ; command ]
        // First, separate command. At this point separator_pos should point to ';'
        // unless null.
        let mut prereq_string = line.clone();
        if let Some(separator_pos) = separator_pos
            && rule_stmt.sep != RuleSep::Semicolon
        {
            assert!(line[separator_pos] == b';');
            let value = line.slice(separator_pos + 1..);
            self.cmds.push(Arc::new(Value::Literal(None, value)));
            prereq_string = line.slice(..separator_pos);
        }

        let Some(separator_pos) = memchr(b':', &prereq_string) else {
            // Simple prerequisites
            self.parse_inputs(&prereq_string);
            return Ok(());
        };

        // Static pattern rule.
        if !self.output_patterns.is_empty() {
            error_loc!(
                Some(&self.loc),
                "*** mixed implicit and normal rules: deprecated syntax"
            );
        }

        // Empty static patterns should not produce rules, but need to eat the
        // commands So return a rule with no outputs nor output_patterns
        if self.outputs.is_empty() {
            return Ok(());
        }

        let target_prereq = prereq_string.slice(..separator_pos);
        let prereq_patterns = prereq_string.slice(separator_pos + 1..);

        for target_pattern in word_scanner(&target_prereq) {
            let target_pattern = target_prereq.slice_ref(trim_leading_curdir(target_pattern));
            let pat = Pattern::new(target_pattern.clone());
            for target in &self.outputs {
                if !pat.matches(&target.as_bytes()) {
                    warn_loc!(
                        Some(&self.loc),
                        "target `{target}' doesn't match the target pattern",
                    );
                }
            }
            self.output_patterns.push(intern(target_pattern));
        }

        if self.output_patterns.is_empty() {
            error_loc!(Some(&self.loc), "*** missing target pattern.");
        }
        if self.output_patterns.len() > 1 {
            error_loc!(Some(&self.loc), "*** multiple target patterns.");
        }
        if !is_pattern_rule(&self.output_patterns.first().unwrap().as_bytes()) {
            error_loc!(Some(&self.loc), "*** target pattern contains no '%'.");
        }
        self.parse_inputs(&prereq_patterns);
        Ok(())
    }
}

impl Debug for Rule {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "outputs={:?} inputs={:?}", self.outputs, self.inputs)?;
        if !self.order_only_inputs.is_empty() {
            write!(f, " order_only_inputs={:?}", self.order_only_inputs)?;
        }
        if !self.output_patterns.is_empty() {
            write!(f, " output_patterns={:?}", self.output_patterns)?;
        }
        if self.is_double_colon {
            write!(f, " is_double_colon")?;
        }
        if self.is_suffix_rule {
            write!(f, " is_suffix_rule")?;
        }
        if !self.cmds.is_empty() {
            write!(f, " cmds={:?}", self.cmds)?;
        }
        Ok(())
    }
}

pub fn is_pattern_rule(target: &[u8]) -> bool {
    memchr(b'%', target).is_some()
}
