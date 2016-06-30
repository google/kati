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

#include "rule.h"

#include "expr.h"
#include "log.h"
#include "parser.h"
#include "stringprintf.h"
#include "strutil.h"
#include "symtab.h"

namespace {

static void ParseInputs(Rule* r, StringPiece s) {
  bool is_order_only = false;
  for (StringPiece input : WordScanner(s)) {
    if (input == "|") {
      is_order_only = true;
      continue;
    }
    Symbol input_sym = Intern(TrimLeadingCurdir(input));
    if (is_order_only) {
      r->order_only_inputs.push_back(input_sym);
    } else {
      r->inputs.push_back(input_sym);
    }
  }
}

bool IsPatternRule(StringPiece s) {
  return s.find('%') != string::npos;
}

}  // namespace

Rule::Rule()
    : is_double_colon(false),
      is_suffix_rule(false),
      cmd_lineno(0) {
}

void ParseRule(Loc& loc, StringPiece line, char term,
               function<string()> after_term_fn,
               Rule** out_rule, RuleVarAssignment* rule_var) {
  size_t index = line.find(':');
  if (index == string::npos) {
    ERROR("%s:%d: *** missing separator.", LOCF(loc));
  }

  StringPiece first = line.substr(0, index);
  vector<Symbol> outputs;
  for (StringPiece tok : WordScanner(first)) {
    outputs.push_back(Intern(TrimLeadingCurdir(tok)));
  }

  const bool is_first_pattern = (
      !outputs.empty() && IsPatternRule(outputs[0].str()));
  for (size_t i = 1; i < outputs.size(); i++) {
    if (IsPatternRule(outputs[i].str()) != is_first_pattern) {
      ERROR("%s:%d: *** mixed implicit and normal rules: deprecated syntax",
            LOCF(loc));
    }
  }

  bool is_double_colon = false;
  index++;
  if (line.get(index) == ':') {
    is_double_colon = true;
    index++;
  }

  StringPiece rest = line.substr(index);
  size_t term_index = rest.find_first_of("=;");
  string buf;
  if ((term_index != string::npos && rest[term_index] == '=') ||
      (term_index == string::npos && term == '=')) {
    if (term_index == string::npos)
      term_index = rest.size();
    // "test: =foo" is questionable but a valid rule definition (not a
    // target specific variable).
    // See https://github.com/google/kati/issues/83
    if (term_index == 0) {
      KATI_WARN("%s:%d: defining a target which starts with `=', "
                "which is not probably what you meant", LOCF(loc));
      buf = line.as_string();
      if (term)
        buf += term;
      buf += after_term_fn();
      line = buf;
      rest = line.substr(index);
      term_index = string::npos;
    } else {
      rule_var->outputs.swap(outputs);
      ParseAssignStatement(rest, term_index,
                           &rule_var->lhs, &rule_var->rhs, &rule_var->op);
      *out_rule = NULL;
      return;
    }
  }

  Rule* rule = new Rule();
  *out_rule = rule;
  rule->loc = loc;
  rule->is_double_colon = is_double_colon;
  if (is_first_pattern) {
    rule->output_patterns.swap(outputs);
  } else {
    rule->outputs.swap(outputs);
  }
  if (term_index != string::npos && term != ';') {
    CHECK(rest[term_index] == ';');
    // TODO: Maybe better to avoid Intern here?
    rule->cmds.push_back(
        NewLiteral(Intern(TrimLeftSpace(rest.substr(term_index + 1))).str()));
    rest = rest.substr(0, term_index);
  }

  index = rest.find(':');
  if (index == string::npos) {
    ParseInputs(rule, rest);
    return;
  }

  if (is_first_pattern) {
    ERROR("%s:%d: *** mixed implicit and normal rules: deprecated syntax",
          LOCF(loc));
  }

  StringPiece second = rest.substr(0, index);
  StringPiece third = rest.substr(index+1);

  for (StringPiece tok : WordScanner(second)) {
    tok = TrimLeadingCurdir(tok);
    for (Symbol output : rule->outputs) {
      if (!Pattern(tok).Match(output.str())) {
        WARN("%s:%d: target `%s' doesn't match the target pattern",
             LOCF(loc), output.c_str());
      }
    }

    rule->output_patterns.push_back(Intern(tok));
  }

  if (rule->output_patterns.empty()) {
    ERROR("%s:%d: *** missing target pattern.", LOCF(loc));
  }
  if (rule->output_patterns.size() > 1) {
    ERROR("%s:%d: *** multiple target patterns.", LOCF(loc));
  }
  if (!IsPatternRule(rule->output_patterns[0].str())) {
    ERROR("%s:%d: *** target pattern contains no '%%'.", LOCF(loc));
  }
  ParseInputs(rule, third);
}

string Rule::DebugString() const {
  vector<string> v;
  v.push_back(StringPrintf("outputs=[%s]", JoinSymbols(outputs, ",").c_str()));
  v.push_back(StringPrintf("inputs=[%s]", JoinSymbols(inputs, ",").c_str()));
  if (!order_only_inputs.empty()) {
    v.push_back(StringPrintf("order_only_inputs=[%s]",
                             JoinSymbols(order_only_inputs, ",").c_str()));
  }
  if (!output_patterns.empty()) {
    v.push_back(StringPrintf("output_patterns=[%s]",
                             JoinSymbols(output_patterns, ",").c_str()));
  }
  if (is_double_colon)
    v.push_back("is_double_colon");
  if (is_suffix_rule)
    v.push_back("is_suffix_rule");
  if (!cmds.empty()) {
    v.push_back(StringPrintf("cmds=[%s]", JoinValues(cmds, ",").c_str()));
  }
  return JoinStrings(v, " ");
}
