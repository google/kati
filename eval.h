#ifndef EVAL_H_
#define EVAL_H_

#include <memory>
#include <unordered_map>
#include <vector>

#include "ast.h"
#include "loc.h"
#include "string_piece.h"

using namespace std;

class Makefile;
class Rule;
class Var;
class Vars;

struct EvalResult {
  ~EvalResult();
  vector<shared_ptr<Rule>> rules;
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

  Vars* mutable_vars() { return vars_; }

  void Error(const string& msg);

  void set_is_bootstrap(bool b) { is_bootstrap_ = b; }

 private:
  void DoAssign(StringPiece lhs, Value* rhs, StringPiece orig_rhs, AssignOp op);
  void DoInclude(const char* fname, bool should_exist);

  const Vars* in_vars_;
  Vars* vars_;
  unordered_map<StringPiece, Vars*> rule_vars_;
  vector<shared_ptr<Rule>> rules_;
  Rule* last_rule_;
  Vars* current_scope_;

  Loc loc_;
  bool is_bootstrap_;
};

#endif  // EVAL_H_
