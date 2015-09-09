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

#include "parser.h"

#include <stack>
#include <unordered_map>

#include "ast.h"
#include "file.h"
#include "loc.h"
#include "log.h"
#include "stats.h"
#include "string_piece.h"
#include "strutil.h"
#include "value.h"

enum struct ParserState {
  NOT_AFTER_RULE = 0,
  AFTER_RULE,
  MAYBE_AFTER_RULE,
};

class Parser {
  struct IfState {
    IfAST* ast;
    bool is_in_else;
    int num_nest;
  };

  typedef void (Parser::*DirectiveHandler)(
      StringPiece line, StringPiece directive);
  typedef unordered_map<StringPiece, DirectiveHandler> DirectiveMap;

 public:
  Parser(StringPiece buf, const char* filename, vector<AST*>* asts)
      : buf_(buf),
        state_(ParserState::NOT_AFTER_RULE),
        asts_(asts),
        out_asts_(asts),
        num_if_nest_(0),
        loc_(filename, 0),
        fixed_lineno_(false) {
  }

  Parser(StringPiece buf, const Loc& loc, vector<AST*>* asts)
      : buf_(buf),
        state_(ParserState::NOT_AFTER_RULE),
        asts_(asts),
        out_asts_(asts),
        num_if_nest_(0),
        loc_(loc),
        fixed_lineno_(true) {
  }

  ~Parser() {
  }

  void Parse() {
    l_ = 0;

    for (l_ = 0; l_ < buf_.size();) {
      size_t lf_cnt = 0;
      size_t e = FindEndOfLine(&lf_cnt);
      if (!fixed_lineno_)
        loc_.lineno++;
      StringPiece line(buf_.data() + l_, e - l_);
      orig_line_with_directives_ = line;
      ParseLine(line);
      if (!fixed_lineno_)
        loc_.lineno += lf_cnt - 1;
      if (e == buf_.size())
        break;

      l_ = e + 1;
    }
  }

  static void Init() {
    make_directives_ = new DirectiveMap;
    (*make_directives_)["include"] = &Parser::ParseInclude;
    (*make_directives_)["-include"] = &Parser::ParseInclude;
    (*make_directives_)["sinclude"] = &Parser::ParseInclude;
    (*make_directives_)["define"] = &Parser::ParseDefine;
    (*make_directives_)["ifdef"] = &Parser::ParseIfdef;
    (*make_directives_)["ifndef"] = &Parser::ParseIfdef;
    (*make_directives_)["ifeq"] = &Parser::ParseIfeq;
    (*make_directives_)["ifneq"] = &Parser::ParseIfeq;
    (*make_directives_)["else"] = &Parser::ParseElse;
    (*make_directives_)["endif"] = &Parser::ParseEndif;
    (*make_directives_)["override"] = &Parser::ParseOverride;
    (*make_directives_)["export"] = &Parser::ParseExport;
    (*make_directives_)["unexport"] = &Parser::ParseUnexport;

    else_if_directives_ = new DirectiveMap;
    (*else_if_directives_)["ifdef"] = &Parser::ParseIfdef;
    (*else_if_directives_)["ifndef"] = &Parser::ParseIfdef;
    (*else_if_directives_)["ifeq"] = &Parser::ParseIfeq;
    (*else_if_directives_)["ifneq"] = &Parser::ParseIfeq;

    assign_directives_ = new DirectiveMap;
    (*assign_directives_)["define"] = &Parser::ParseDefine;
    (*assign_directives_)["export"] = &Parser::ParseExport;
    (*assign_directives_)["override"] = &Parser::ParseOverride;

    shortest_directive_len_ = 9999;
    longest_directive_len_ = 0;
    for (auto p : *make_directives_) {
      size_t len = p.first.size();
      shortest_directive_len_ = min(len, shortest_directive_len_);
      longest_directive_len_ = max(len, longest_directive_len_);
    }
  }

  static void Quit() {
    delete make_directives_;
  }

  void set_state(ParserState st) { state_ = st; }

  static vector<ParseErrorAST*> parse_errors;

 private:
  void Error(const string& msg) {
    ParseErrorAST* ast = new ParseErrorAST();
    ast->set_loc(loc_);
    ast->msg = msg;
    out_asts_->push_back(ast);
    parse_errors.push_back(ast);
  }

  size_t FindEndOfLine(size_t* lf_cnt) {
    return ::FindEndOfLine(buf_, l_, lf_cnt);
  }

  Value* ParseExpr(StringPiece s, ParseExprOpt opt = ParseExprOpt::NORMAL) {
    return ::ParseExpr(loc_, s, opt);
  }

  void ParseLine(StringPiece line) {
    if (!define_name_.empty()) {
      ParseInsideDefine(line);
      return;
    }

    if (line.empty() || (line.size() == 1 && line[0] == '\r'))
      return;

    current_directive_ = AssignDirective::NONE;

    if (line[0] == '\t' && state_ != ParserState::NOT_AFTER_RULE) {
      CommandAST* ast = new CommandAST();
      ast->set_loc(loc_);
      ast->expr = ParseExpr(line.substr(1), ParseExprOpt::COMMAND);
      ast->orig = line;
      out_asts_->push_back(ast);
      return;
    }

    line = TrimLeftSpace(line);

    if (line[0] == '#')
      return;

    if (HandleDirective(line, make_directives_)) {
      return;
    }

    ParseRuleOrAssign(line);
  }

  void ParseRuleOrAssign(StringPiece line) {
    size_t sep = FindThreeOutsideParen(line, ':', '=', ';');
    if (sep == string::npos || line[sep] == ';') {
      ParseRule(line, string::npos);
    } else if (line[sep] == '=') {
      ParseAssign(line, sep);
    } else if (line.get(sep+1) == '=') {
      ParseAssign(line, sep+1);
    } else if (line[sep] == ':') {
      ParseRule(line, sep);
    } else {
      CHECK(false);
    }
  }

  void ParseRule(StringPiece line, size_t sep) {
    if (current_directive_ != AssignDirective::NONE) {
      if (IsInExport())
        return;
      if (sep != string::npos) {
        sep += orig_line_with_directives_.size() - line.size();
      }
      line = orig_line_with_directives_;
    }

    line = TrimLeftSpace(line);
    if (line.empty())
      return;

    if (orig_line_with_directives_[0] == '\t') {
      Error("*** commands commence before first target.");
      return;
    }

    const bool is_rule = sep != string::npos && line[sep] == ':';
    RuleAST* ast = new RuleAST();
    ast->set_loc(loc_);

    size_t found = FindTwoOutsideParen(line.substr(sep + 1), '=', ';');
    if (found != string::npos) {
      found += sep + 1;
      ast->term = line[found];
      ParseExprOpt opt =
          ast->term == ';' ? ParseExprOpt::COMMAND : ParseExprOpt::NORMAL;
      ast->after_term = ParseExpr(TrimLeftSpace(line.substr(found + 1)), opt);
      ast->expr = ParseExpr(TrimSpace(line.substr(0, found)));
    } else {
      ast->term = 0;
      ast->after_term = NULL;
      ast->expr = ParseExpr(line);
    }
    out_asts_->push_back(ast);
    state_ = is_rule ? ParserState::AFTER_RULE : ParserState::MAYBE_AFTER_RULE;
  }

  void ParseAssign(StringPiece line, size_t sep) {
    if (sep == 0) {
      Error("*** empty variable name ***");
      return;
    }
    StringPiece lhs;
    StringPiece rhs;
    AssignOp op;
    ParseAssignStatement(line, sep, &lhs, &rhs, &op);

    AssignAST* ast = new AssignAST();
    ast->set_loc(loc_);
    ast->lhs = ParseExpr(lhs);
    ast->rhs = ParseExpr(rhs);
    ast->orig_rhs = rhs;
    ast->op = op;
    ast->directive = current_directive_;
    out_asts_->push_back(ast);
    state_ = ParserState::NOT_AFTER_RULE;
  }

  void ParseInclude(StringPiece line, StringPiece directive) {
    IncludeAST* ast = new IncludeAST();
    ast->set_loc(loc_);
    ast->expr = ParseExpr(line);
    ast->should_exist = directive[0] == 'i';
    out_asts_->push_back(ast);
    state_ = ParserState::NOT_AFTER_RULE;
  }

  void ParseDefine(StringPiece line, StringPiece) {
    if (line.empty()) {
      Error("*** empty variable name.");
      return;
    }
    define_name_ = line;
    define_start_ = 0;
    define_start_line_ = loc_.lineno;
    state_ = ParserState::NOT_AFTER_RULE;
  }

  void ParseInsideDefine(StringPiece line) {
    line = TrimLeftSpace(line);
    if (GetDirective(line) != "endef") {
      if (define_start_ == 0)
        define_start_ = l_;
      return;
    }

    StringPiece rest = TrimRightSpace(RemoveComment(TrimLeftSpace(
        line.substr(sizeof("endef")))));
    if (!rest.empty()) {
      WARN("%s:%d: extraneous text after `endef' directive", LOCF(loc_));
    }

    AssignAST* ast = new AssignAST();
    ast->set_loc(Loc(loc_.filename, define_start_line_));
    ast->lhs = ParseExpr(define_name_);
    StringPiece rhs;
    if (define_start_)
      rhs = buf_.substr(define_start_, l_ - define_start_ - 1);
    ast->rhs = ParseExpr(rhs, ParseExprOpt::DEFINE);
    ast->orig_rhs = rhs;
    ast->op = AssignOp::EQ;
    ast->directive = current_directive_;
    out_asts_->push_back(ast);
    define_name_.clear();
  }

  void EnterIf(IfAST* ast) {
    IfState* st = new IfState();
    st->ast = ast;
    st->is_in_else = false;
    st->num_nest = num_if_nest_;
    if_stack_.push(st);
    out_asts_ = &ast->true_asts;
  }

  void ParseIfdef(StringPiece line, StringPiece directive) {
    IfAST* ast = new IfAST();
    ast->set_loc(loc_);
    ast->op = directive[2] == 'n' ? CondOp::IFNDEF : CondOp::IFDEF;
    ast->lhs = ParseExpr(line);
    ast->rhs = NULL;
    out_asts_->push_back(ast);
    EnterIf(ast);
  }

  bool ParseIfEqCond(StringPiece s, IfAST* ast) {
    if (s.empty()) {
      return false;
    }

    if (s[0] == '(' && s[s.size() - 1] == ')') {
      s = s.substr(1, s.size() - 2);
      char terms[] = {',', '\0'};
      size_t n;
      ast->lhs = ParseExprImpl(loc_, s, terms, ParseExprOpt::NORMAL, &n, true);
      if (s[n] != ',')
        return false;
      s = TrimLeftSpace(s.substr(n+1));
      ast->rhs = ParseExprImpl(loc_, s, NULL, ParseExprOpt::NORMAL, &n);
      s = TrimLeftSpace(s.substr(n));
    } else {
      for (int i = 0; i < 2; i++) {
        if (s.empty())
          return false;
        char quote = s[0];
        if (quote != '\'' && quote != '"')
          return false;
        size_t end = s.find(quote, 1);
        if (end == string::npos)
          return false;
        Value* v = ParseExpr(s.substr(1, end - 1), ParseExprOpt::NORMAL);
        if (i == 0)
          ast->lhs = v;
        else
          ast->rhs = v;
        s = TrimLeftSpace(s.substr(end+1));
      }
    }
    if (!s.empty()) {
      WARN("%s:%d: extraneous text after `ifeq' directive", LOCF(loc_));
      return true;
    }
    return true;
  }

  void ParseIfeq(StringPiece line, StringPiece directive) {
    IfAST* ast = new IfAST();
    ast->set_loc(loc_);
    ast->op = directive[2] == 'n' ? CondOp::IFNEQ : CondOp::IFEQ;

    if (!ParseIfEqCond(line, ast)) {
      Error("*** invalid syntax in conditional.");
      return;
    }

    out_asts_->push_back(ast);
    EnterIf(ast);
  }

  void ParseElse(StringPiece line, StringPiece) {
    if (!CheckIfStack("else"))
      return;
    IfState* st = if_stack_.top();
    if (st->is_in_else) {
      Error("*** only one `else' per conditional.");
      return;
    }
    st->is_in_else = true;
    out_asts_ = &st->ast->false_asts;

    StringPiece next_if = TrimLeftSpace(line);
    if (next_if.empty())
      return;

    num_if_nest_ = st->num_nest + 1;
    if (!HandleDirective(next_if, else_if_directives_)) {
      WARN("%s:%d: extraneous text after `else' directive", LOCF(loc_));
    }
    num_if_nest_ = 0;
  }

  void ParseEndif(StringPiece line, StringPiece) {
    if (!CheckIfStack("endif"))
      return;
    if (!line.empty()) {
      Error("extraneous text after `endif` directive");
      return;
    }
    IfState st = *if_stack_.top();
    for (int t = 0; t <= st.num_nest; t++) {
      delete if_stack_.top();
      if_stack_.pop();
      if (if_stack_.empty()) {
        out_asts_ = asts_;
      } else {
        IfState* st = if_stack_.top();
        if (st->is_in_else)
          out_asts_ = &st->ast->false_asts;
        else
          out_asts_ = &st->ast->true_asts;
      }
    }
  }

  bool IsInExport() const {
    return (static_cast<int>(current_directive_) &
            static_cast<int>(AssignDirective::EXPORT));
  }

  void CreateExport(StringPiece line, bool is_export) {
    ExportAST* ast = new ExportAST;
    ast->set_loc(loc_);
    ast->expr = ParseExpr(line);
    ast->is_export = is_export;
    out_asts_->push_back(ast);
  }

  void ParseOverride(StringPiece line, StringPiece) {
    current_directive_ =
        static_cast<AssignDirective>(
            (static_cast<int>(current_directive_) |
             static_cast<int>(AssignDirective::OVERRIDE)));
    if (HandleDirective(line, assign_directives_))
      return;
    if (IsInExport()) {
      CreateExport(line, true);
    }
    ParseRuleOrAssign(line);
  }

  void ParseExport(StringPiece line, StringPiece) {
    current_directive_ =
        static_cast<AssignDirective>(
            (static_cast<int>(current_directive_) |
             static_cast<int>(AssignDirective::EXPORT)));
    if (HandleDirective(line, assign_directives_))
      return;
    CreateExport(line, true);
    ParseRuleOrAssign(line);
  }

  void ParseUnexport(StringPiece line, StringPiece) {
    CreateExport(line, false);
  }

  bool CheckIfStack(const char* keyword) {
    if (if_stack_.empty()) {
      Error(StringPrintf("*** extraneous `%s'.", keyword));
      return false;
    }
    return true;
  }

  StringPiece RemoveComment(StringPiece line) {
    size_t i = FindOutsideParen(line, '#');
    if (i == string::npos)
      return line;
    return line.substr(0, i);
  }

  StringPiece GetDirective(StringPiece line) {
    if (line.size() < shortest_directive_len_)
      return StringPiece();
    StringPiece prefix = line.substr(0, longest_directive_len_ + 1);
    size_t space_index = prefix.find_first_of(" \t#");
    return prefix.substr(0, space_index);
  }

  bool HandleDirective(StringPiece line, const DirectiveMap* directive_map) {
    StringPiece directive = GetDirective(line);
    auto found = directive_map->find(directive);
    if (found == directive_map->end())
      return false;

    StringPiece rest = TrimRightSpace(RemoveComment(TrimLeftSpace(
        line.substr(directive.size()))));
    (this->*found->second)(rest, directive);
    return true;
  }

  StringPiece buf_;
  size_t l_;
  ParserState state_;

  vector<AST*>* asts_;
  vector<AST*>* out_asts_;

  StringPiece define_name_;
  size_t define_start_;
  int define_start_line_;

  StringPiece orig_line_with_directives_;
  AssignDirective current_directive_;

  int num_if_nest_;
  stack<IfState*> if_stack_;

  Loc loc_;
  bool fixed_lineno_;

  static DirectiveMap* make_directives_;
  static DirectiveMap* else_if_directives_;
  static DirectiveMap* assign_directives_;
  static size_t shortest_directive_len_;
  static size_t longest_directive_len_;
};

void Parse(Makefile* mk) {
  COLLECT_STATS("parse file time");
  Parser parser(StringPiece(mk->buf(), mk->len()),
                mk->filename().c_str(),
                mk->mutable_asts());
  parser.Parse();
}

void Parse(StringPiece buf, const Loc& loc, vector<AST*>* out_asts) {
  COLLECT_STATS("parse eval time");
  Parser parser(buf, loc, out_asts);
  parser.Parse();
}

void ParseNotAfterRule(StringPiece buf, const Loc& loc,
                       vector<AST*>* out_asts) {
  Parser parser(buf, loc, out_asts);
  parser.set_state(ParserState::NOT_AFTER_RULE);
  parser.Parse();
}

void InitParser() {
  Parser::Init();
}

void QuitParser() {
  Parser::Quit();
}

Parser::DirectiveMap* Parser::make_directives_;
Parser::DirectiveMap* Parser::else_if_directives_;
Parser::DirectiveMap* Parser::assign_directives_;
size_t Parser::shortest_directive_len_;
size_t Parser::longest_directive_len_;
vector<ParseErrorAST*> Parser::parse_errors;

void ParseAssignStatement(StringPiece line, size_t sep,
                          StringPiece* lhs, StringPiece* rhs, AssignOp* op) {
  CHECK(sep != 0);
  *op = AssignOp::EQ;
  size_t lhs_end = sep;
  switch (line[sep-1]) {
    case ':':
      lhs_end--;
      *op = AssignOp::COLON_EQ;
      break;
    case '+':
      lhs_end--;
      *op = AssignOp::PLUS_EQ;
      break;
    case '?':
      lhs_end--;
      *op = AssignOp::QUESTION_EQ;
      break;
  }
  *lhs = TrimSpace(line.substr(0, lhs_end));
  *rhs = TrimSpace(line.substr(sep + 1));
}

const vector<ParseErrorAST*>& GetParseErrors() {
  return Parser::parse_errors;
}
