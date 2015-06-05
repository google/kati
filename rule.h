#ifndef RULE_H_
#define RULE_H_

#include <vector>

#include "loc.h"
#include "log.h"
#include "string_piece.h"

using namespace std;

class Value;

class Rule {
 public:
  Rule();
  void Parse(StringPiece line);

  string DebugString() const;

  vector<StringPiece> outputs;
  vector<StringPiece> inputs;
  vector<StringPiece> order_only_inputs;
  vector<string> output_patterns;
  bool is_double_colon;
  bool is_suffix_rule;
  vector<Value*> cmds;
  Loc loc;
  int cmd_lineno;

 private:
  void Error(const string& msg) {
    ERROR("%s:%d: %s", loc.filename, loc.lineno, msg.c_str());
  }
};

#endif  // RULE_H_
