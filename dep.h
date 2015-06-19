#ifndef DEP_H_
#define DEP_H_

#include <memory>
#include <string>
#include <unordered_map>
#include <vector>

#include "loc.h"
#include "string_piece.h"

class Rule;
class Value;
class Vars;

struct DepNode {
  DepNode(StringPiece output, bool is_phony);

  StringPiece output;
  vector<Value*> cmds;
  vector<DepNode*> deps;
  vector<DepNode*> parents;
  bool has_rule;
  bool is_order_only;
  bool is_phony;
  vector<StringPiece> actual_inputs;
  Vars* target_specific_vars;
  Loc loc;
};

void InitDepNodePool();
void QuitDepNodePool();

void MakeDep(const vector<shared_ptr<Rule>>& rules,
             const Vars& vars,
             const unordered_map<StringPiece, Vars*>& rule_vars,
             const vector<StringPiece>& targets,
             vector<DepNode*>* nodes);

#endif  // DEP_H_
