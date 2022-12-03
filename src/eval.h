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

#ifndef EVAL_H_
#define EVAL_H_

#include <map>
#include <memory>
#include <set>
#include <string_view>
#include <unordered_map>
#include <unordered_set>
#include <vector>

#include "loc.h"
#include "stmt.h"
#include "symtab.h"

class Makefile;
class Rule;
class Var;
class Vars;

class IncludeGraph;

enum RulesAllowed { RULES_ALLOWED = 0, RULES_WARNING = 1, RULES_ERROR = 2 };

enum FrameType {
  ROOT,        // Root node. Exactly one of this exists.
  PHASE,       // Markers for various phases of the execution.
  PARSE,       // Initial evaluation pass: include, := variables, etc.
  CALL,        // Evaluating the result of a function call
  FUNCALL,     // Evaluating a function call (not its result)
  STATEMENT,   // Denotes individual statements for better location reporting
  DEPENDENCY,  // Dependency analysis. += requires variable expansion here.
  EXEC,        // Execution phase. Expansion of = and rule-specific variables.
  NINJA,       // Ninja file generation
};

class Frame {
 public:
  // for the top-level Makefile
  Frame(FrameType type, Frame* parent, Loc loc, const std::string& name);

  ~Frame();

  FrameType Type() const { return type_; }
  Frame* Parent() const { return parent_; }
  const std::string& Name() const { return name_; }
  const Loc& Location() const { return location_; }
  const std::vector<std::unique_ptr<Frame>>& Children() const {
    return children_;
  }

  void PrintJSONTrace(FILE* f, int indent) const;

  void Add(std::unique_ptr<Frame> child);

 private:
  FrameType type_;
  Frame* parent_;
  std::string name_;
  Loc location_;
  std::vector<std::unique_ptr<Frame>> children_;
};

class ScopedFrame {
 public:
  ScopedFrame(Evaluator* ev, Frame* frame);
  // We disallow any copies or moves of this object.
  ScopedFrame(const ScopedFrame& other) = delete;
  ScopedFrame& operator=(const ScopedFrame&) = delete;
  ScopedFrame(ScopedFrame&& other) = delete;
  ~ScopedFrame();

  Frame* Current() const { return frame_; }

 private:
  Evaluator* ev_;
  Frame* frame_;
};

class IncludeGraphNode {
  friend IncludeGraph;

 public:
  IncludeGraphNode(const Frame* frame);
  ~IncludeGraphNode();

 private:
  std::string filename_;
  std::set<std::string> includes_;
};

class IncludeGraph {
 public:
  IncludeGraph();
  ~IncludeGraph();

  void DumpJSON(FILE* output);
  void MergeTreeNode(const Frame* frame);

 private:
  std::map<std::string, std::unique_ptr<IncludeGraphNode>> nodes_;
  std::vector<const Frame*> include_stack_;
};

class Evaluator {
  friend ScopedFrame;

 public:
  Evaluator();
  ~Evaluator();

  bool Start();
  void Finish();

  void EvalAssign(const AssignStmt* stmt);
  void EvalRule(const RuleStmt* stmt);
  void EvalCommand(const CommandStmt* stmt);
  void EvalIf(const IfStmt* stmt);
  void EvalInclude(const IncludeStmt* stmt);
  void EvalExport(const ExportStmt* stmt);

  Var* LookupVarForEval(Symbol name);
  Var* LookupVar(Symbol name);
  // For target specific variables.
  Var* LookupVarInCurrentScope(Symbol name);
  void VarEvalComplete(Symbol name);

  // Equivalent to LookupVar, but doesn't mark as used.
  Var* PeekVar(Symbol name);

  std::string EvalVar(Symbol name);

  const Loc& loc() const { return loc_; }
  void set_loc(const Loc& loc) { loc_ = loc; }

  const std::vector<const Rule*>& rules() const { return rules_; }
  const std::unordered_map<Symbol, Vars*>& rule_vars() const {
    return rule_vars_;
  }
  const std::unordered_map<Symbol, bool>& exports() const { return exports_; }

  void PrintIncludeStack();
  void Error(const std::string& msg);

  void in_bootstrap();
  void in_command_line();
  void in_toplevel_makefile();

  void set_current_scope(Vars* v) { current_scope_ = v; }

  bool avoid_io() const { return avoid_io_; }
  void set_avoid_io(bool a) { avoid_io_ = a; }

  const std::vector<std::string>& delayed_output_commands() const {
    return delayed_output_commands_;
  }
  void add_delayed_output_command(const std::string& c) {
    delayed_output_commands_.push_back(c);
  }
  void clear_delayed_output_commands() { delayed_output_commands_.clear(); }

  static const SymbolSet& used_undefined_vars() { return used_undefined_vars_; }

  int eval_depth() const { return eval_depth_; }
  void IncrementEvalDepth() { eval_depth_++; }
  void DecrementEvalDepth() { eval_depth_--; }

  ScopedFrame Enter(FrameType frame_type, const std::string& name, Loc loc);
  Frame* CurrentFrame() const {
    return stack_.empty() ? nullptr : stack_.back();
  };

  std::string GetShell();
  std::string GetShellFlag();
  std::string GetShellAndFlag();

  // Parse the current value of .KATI_ALLOW_RULES
  RulesAllowed GetAllowRules();

  void CheckStack() {
    void* addr = __builtin_frame_address(0);
    if (__builtin_expect(addr < lowest_stack_ && addr >= stack_addr_, 0)) {
      lowest_stack_ = addr;
      lowest_loc_ = loc_;
    }
  }

  void DumpStackStats() const;
  void DumpIncludeJSON(const std::string& filename) const;

  bool ExportDeprecated() const { return export_message_ && !export_error_; };
  bool ExportObsolete() const { return export_error_; };
  void SetExportDeprecated(std::string_view msg) {
    export_message_.reset(new std::string(msg));
  }
  void SetExportObsolete(std::string_view msg) {
    export_message_.reset(new std::string(msg));
    export_error_ = true;
  }

  void ProfileMakefile(std::string_view mk) {
    profiled_files_.emplace_back(std::string(mk));
  }

  bool IsEvaluatingCommand() const;
  void SetEvaluatingCommand(bool evaluating_command);

 private:
  Var* EvalRHS(Symbol lhs,
               Value* rhs,
               std::string_view orig_rhs,
               AssignOp op,
               bool is_override,
               bool* needs_assign);
  void DoInclude(const std::string& fname);

  bool IsTraced(Symbol& name) const;
  void TraceVariableLookup(const char* operation, Symbol& name, Var* var);
  void TraceVariableAssign(Symbol& name, Var* var);
  Var* LookupVarGlobal(Symbol name);

  // Equivalent to LookupVarInCurrentScope, but doesn't mark as used.
  Var* PeekVarInCurrentScope(Symbol name);

  void MarkVarsReadonly(Value* var_list);

  void EvalRuleSpecificAssign(const std::vector<Symbol>& targets,
                              const RuleStmt* stmt,
                              const std::string_view& lhs_string,
                              size_t separator_pos);

  std::unordered_map<Symbol, Vars*> rule_vars_;
  std::vector<const Rule*> rules_;
  std::unordered_map<Symbol, bool> exports_;
  std::set<Symbol> symbols_for_eval_;

  Rule* last_rule_;
  Vars* current_scope_;

  Loc loc_;
  bool is_bootstrap_;
  bool is_commandline_;

  bool trace_;
  std::vector<Frame*> stack_;
  FILE* assignment_tracefile_;
  const char* assignment_sep_;

  std::vector<Loc> include_stack_;

  bool avoid_io_;
  // This value tracks the nest level of make expressions. For
  // example, $(YYY) in $(XXX $(YYY)) is evaluated with depth==2.
  // This will be used to disallow $(shell) in other make constructs.
  int eval_depth_;
  // Commands which should run at ninja-time (i.e., info, warning, and
  // error).
  std::vector<std::string> delayed_output_commands_;

  Symbol posix_sym_;
  bool is_posix_;

  void* stack_addr_;
  size_t stack_size_;
  void* lowest_stack_;
  Loc lowest_loc_;

  std::unique_ptr<std::string> export_message_;
  bool export_error_;

  std::vector<std::string> profiled_files_;

  static SymbolSet used_undefined_vars_;

  bool is_evaluating_command_ = false;
};

#endif  // EVAL_H_
