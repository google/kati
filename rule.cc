#include "rule.h"

#include "log.h"
#include "parser.h"
#include "stringprintf.h"
#include "strutil.h"
#include "value.h"

namespace {

// Strip leading sequences of './' from file names, so that ./file
// and file are considered to be the same file.
// From http://www.gnu.org/software/make/manual/make.html#Features
StringPiece TrimLeadingCurdir(StringPiece s) {
  if (s.substr(0, 2) != "./")
    return s;
  return s.substr(2);
}

static void ParseInputs(Rule* r, StringPiece s) {
  bool is_order_only = false;
  for (StringPiece input : WordScanner(s)) {
    if (input == "|") {
      is_order_only = true;
      continue;
    }
    input = Intern(TrimLeadingCurdir(input));
    if (is_order_only) {
      r->order_only_inputs.push_back(input);
    } else {
      r->inputs.push_back(input);
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

void ParseRule(Loc& loc, StringPiece line, bool is_assign,
               Rule** out_rule, RuleVar* rule_var) {
  size_t index = line.find(':');
  if (index == string::npos) {
    ERROR("%s:%d: *** missing separator.", LOCF(loc));
  }

  StringPiece first = line.substr(0, index);
  vector<StringPiece> outputs;
  for (StringPiece tok : WordScanner(first)) {
    outputs.push_back(Intern(TrimLeadingCurdir(tok)));
  }

  CHECK(!outputs.empty());
  const bool is_first_pattern = IsPatternRule(outputs[0]);
  if (is_first_pattern) {
    if (outputs.size() > 1) {
      // TODO: Multiple output patterns are not supported yet.
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
  size_t equal_index = rest.find('=');
  if (equal_index != string::npos || is_assign) {
    if (equal_index == string::npos)
      equal_index = rest.size();
    rule_var->outputs.swap(outputs);
    ParseAssignStatement(rest, equal_index,
                         &rule_var->lhs, &rule_var->rhs, &rule_var->op);
    *out_rule = NULL;
    return;
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
    rule->output_patterns.push_back(tok);
  }

  if (rule->output_patterns.empty()) {
    ERROR("%s:%d: *** missing target pattern.", LOCF(loc));
  }
  if (rule->output_patterns.size() > 1) {
    ERROR("%s:%d: *** multiple target patterns.", LOCF(loc));
  }
  if (!IsPatternRule(rule->output_patterns[0])) {
    ERROR("%s:%d: *** target pattern contains no '%%'.", LOCF(loc));
  }
  ParseInputs(rule, third);
}

string Rule::DebugString() const {
  vector<string> v;
  v.push_back(StringPrintf("outputs=[%s]", JoinStrings(outputs, ",").c_str()));
  v.push_back(StringPrintf("inputs=[%s]", JoinStrings(inputs, ",").c_str()));
  if (!order_only_inputs.empty()) {
    v.push_back(StringPrintf("order_only_inputs=[%s]",
                             JoinStrings(order_only_inputs, ",").c_str()));
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
