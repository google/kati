#include "rule.h"

#include "log.h"
#include "stringprintf.h"
#include "strutil.h"
#include "value.h"

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

Rule::Rule()
    : is_double_colon(false),
      is_suffix_rule(false),
      cmd_lineno(0) {
}

void Rule::Parse(StringPiece line) {
  size_t index = line.find(':');
  if (index == string::npos) {
    Error("*** missing separator.");
  }

  StringPiece first = line.substr(0, index);
  // TODO: isPattern?
  if (false) {
  } else {
    for (StringPiece tok : WordScanner(first)) {
      outputs.push_back(Intern(TrimLeadingCurdir(tok)));
    }
  }

  index++;
  if (line.get(index) == ':') {
    is_double_colon = true;
    index++;
  }

  StringPiece rest = line.substr(index);

  // TODO: TSV
  //if (

  index = rest.find(':');
  if (index == string::npos) {
    ParseInputs(this, rest);
    return;
  }
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
