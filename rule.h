#ifndef RULE_H_
#define RULE_H_

#include <vector>

#include "ast.h"
#include "loc.h"
#include "log.h"
#include "string_piece.h"

using namespace std;

class Evaluator;
class Value;

class Rule {
 public:
  Rule();

  string DebugString() const;

  vector<StringPiece> outputs;
  vector<StringPiece> inputs;
  vector<StringPiece> order_only_inputs;
  vector<StringPiece> output_patterns;
  bool is_double_colon;
  bool is_suffix_rule;
  vector<Value*> cmds;
  Loc loc;
  int cmd_lineno;
  bool is_temporary;

 private:
  void Error(const string& msg) {
    ERROR("%s:%d: %s", loc.filename, loc.lineno, msg.c_str());
  }
};

struct RuleVar {
  vector<StringPiece> outputs;
  StringPiece lhs;
  StringPiece rhs;
  AssignOp op;
};

// If |rule| is not NULL, rule_var is filled.
void ParseRule(Loc& loc, StringPiece line, Rule** rule, RuleVar* rule_var);

#endif  // RULE_H_
