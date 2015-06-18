#ifndef EVAL_H_
#define EVAL_H_

#include <unordered_map>
#include <vector>

#include "loc.h"
#include "string_piece.h"

using namespace std;

class AssignAST;
class CommandAST;
class ExportAST;
class IfAST;
class IncludeAST;
class Makefile;
class Rule;
class RuleAST;
class Var;
class Vars;

struct EvalResult {
  ~EvalResult();
  vector<Rule*> rules;
  Vars* vars;
  unordered_map<StringPiece, Vars*> rule_vars;
  // TODO: read_mks
  unordered_map<StringPiece, bool> exports;
};

class Evaluator {
 public:
  Evaluator(const Vars* vars);
  ~Evaluator();

  void EvalAssign(const AssignAST* ast);
  void EvalRule(const RuleAST* ast);
  void EvalCommand(const CommandAST* ast);
  void EvalIf(const IfAST* ast);
  void EvalInclude(const IncludeAST* ast);
  void EvalExport(const ExportAST* ast);


  Var* LookupVar(StringPiece name);
  // For target specific variables.
  Var* LookupVarInCurrentScope(StringPiece name);

  EvalResult* GetEvalResult();

  const Loc& loc() const { return loc_; }

#if 0
  const vector<Rule*>& rules() const { return rules_; }
  const Vars* vars() const { return vars_; }
  const unordered_map<StringPiece, Vars*>& rule_vars() const {
    return rule_vars_;
  }
#endif

  void Error(const string& msg);

 private:
  void DoInclude(const char* fname, bool should_exist);

  const Vars* in_vars_;
  Vars* vars_;
  unordered_map<StringPiece, Vars*> rule_vars_;
  vector<Rule*> rules_;
  Rule* last_rule_;

  Loc loc_;
};

#endif  // EVAL_H_
