#include "parser.h"

#include <stack>
#include <unordered_map>

#include "ast.h"
#include "file.h"
#include "loc.h"
#include "log.h"
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

  ~Parser() {
  }

  void Parse() {
    l_ = 0;

    for (l_ = 0; l_ < buf_.size();) {
      size_t lf_cnt = 0;
      size_t e = FindEndOfLine(&lf_cnt);
      if (!fixed_lineno_)
        loc_.lineno += lf_cnt;
      StringPiece line(buf_.data() + l_, e - l_);
      ParseLine(line);
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

    else_if_directives_ = new DirectiveMap;
    (*else_if_directives_)["ifdef"] = &Parser::ParseIfdef;
    (*else_if_directives_)["ifndef"] = &Parser::ParseIfdef;

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

 private:
  void Error(const string& msg) {
    ERROR("%s:%d: %s", LOCF(loc_), msg.c_str());
  }

  size_t FindEndOfLine(size_t* lf_cnt) {
    size_t e = l_;
    bool prev_backslash = false;
    for (; e < buf_.size(); e++) {
      char c = buf_[e];
      if (c == '\\') {
        prev_backslash = !prev_backslash;
      } else if (c == '\n') {
        ++*lf_cnt;
        if (!prev_backslash) {
          return e;
        }
      } else if (c != '\r') {
        prev_backslash = false;
      }
    }
    return e;
  }

  void ParseLine(StringPiece line) {
    if (line.empty() || (line.size() == 1 && line[0] == '\r'))
      return;

    if (!define_name_.empty()) {
      ParseInsideDefine(line);
      return;
    }

    if (line[0] == '\t' && state_ != ParserState::NOT_AFTER_RULE) {
      CommandAST* ast = new CommandAST();
      ast->expr = ParseExpr(line.substr(1), true);
      out_asts_->push_back(ast);
      return;
    }

    line = TrimLeftSpace(line);

    if (line[0] == '#')
      return;

    if (HandleDirective(line, make_directives_)) {
      return;
    }

    size_t sep = line.find_first_of(STRING_PIECE("=:"));
    if (sep == string::npos) {
      ParseRule(line, sep);
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
    const bool is_rule = line.find(':') != string::npos;
    RuleAST* ast = new RuleAST();
    ast->set_loc(loc_);

    size_t found = line.substr(sep + 1).find_first_of("=;");
    if (found != string::npos) {
      found += sep + 1;
      ast->term = line[found];
      ast->after_term = ParseExpr(TrimLeftSpace(line.substr(found + 1)),
                                  ast->term == ';');
      ast->expr = ParseExpr(TrimSpace(line.substr(0, found)), false);
    } else {
      ast->term = 0;
      ast->after_term = NULL;
      ast->expr = ParseExpr(TrimSpace(line), false);
    }
    out_asts_->push_back(ast);
    state_ = is_rule ? ParserState::AFTER_RULE : ParserState::MAYBE_AFTER_RULE;
  }

  void ParseAssign(StringPiece line, size_t sep) {
    if (sep == 0)
      Error("*** empty variable name ***");
    AssignOp op = AssignOp::EQ;
    size_t lhs_end = sep;
    switch (line[sep-1]) {
      case ':':
        lhs_end--;
        op = AssignOp::COLON_EQ;
        break;
      case '+':
        lhs_end--;
        op = AssignOp::PLUS_EQ;
        break;
      case '?':
        lhs_end--;
        op = AssignOp::QUESTION_EQ;
        break;
    }

    AssignAST* ast = new AssignAST();
    ast->set_loc(loc_);
    ast->lhs = ParseExpr(TrimSpace(line.substr(0, lhs_end)), false);
    ast->rhs = ParseExpr(TrimSpace(line.substr(sep + 1)), false);
    ast->op = op;
    ast->directive = AssignDirective::NONE;
    out_asts_->push_back(ast);
    state_ = ParserState::NOT_AFTER_RULE;
  }

  void ParseInclude(StringPiece line, StringPiece directive) {
    IncludeAST* ast = new IncludeAST();
    ast->expr = ParseExpr(line, false);
    ast->should_exist = directive[0] == 'i';
    out_asts_->push_back(ast);
  }

  void ParseDefine(StringPiece line, StringPiece) {
    if (line.empty()) {
      Error("*** empty variable name.");
    }
    define_name_ = line;
    define_start_ = 0;
    define_start_line_ = loc_.lineno;
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
    ast->lhs = ParseExpr(define_name_, false);
    StringPiece rhs;
    if (define_start_)
      rhs = TrimRightSpace(buf_.substr(define_start_, l_ - define_start_));
    ast->rhs = ParseExpr(rhs, false);
    ast->op = AssignOp::EQ;
    ast->directive = AssignDirective::NONE;
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
    ast->lhs = ParseExpr(line, false);
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
      ast->lhs = ParseExprImpl(s, terms, false, &n, true);
      if (s[n] != ',')
        return false;
      s = TrimLeftSpace(s.substr(n+1));
      ast->rhs = ParseExprImpl(s, NULL, false, &n);
      return TrimSpace(s.substr(n)) == "";
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
        Value* v = ParseExpr(s.substr(1, end - 1), false);
        if (i == 0)
          ast->lhs = v;
        else
          ast->rhs = v;
        s = TrimLeftSpace(s.substr(end+1));
      }
      return s.empty();
    }
  }

  void ParseIfeq(StringPiece line, StringPiece directive) {
    IfAST* ast = new IfAST();
    ast->set_loc(loc_);
    ast->op = directive[2] == 'n' ? CondOp::IFNEQ : CondOp::IFEQ;

    if (!ParseIfEqCond(line, ast)) {
      Error("*** invalid syntax in conditional.");
    }

    out_asts_->push_back(ast);
    EnterIf(ast);
  }

  void ParseElse(StringPiece line, StringPiece) {
    CheckIfStack("else");
    IfState* st = if_stack_.top();
    if (st->is_in_else)
      Error("*** only one `else' per conditional.");
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
    CheckIfStack("endif");
    if (!line.empty())
      Error("extraneous text after `endif` directive");
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

  void CheckIfStack(const char* keyword) {
    if (if_stack_.empty()) {
      Error(StringPrintf("*** extraneous `%s'.", keyword));
    }
  }

  StringPiece RemoveComment(StringPiece line) {
    bool prev_backslash = false;
    stack<char> paren_stack;
    for (size_t i = 0; i < line.size(); i++) {
      char c = line[i];
      switch (c) {
        case '(':
          paren_stack.push(')');
          break;
        case '{':
          paren_stack.push('}');
          break;

        case ')':
        case '}':
          if (!paren_stack.empty() && c == paren_stack.top()) {
            paren_stack.pop();
          }
          break;

        case '#':
          if (paren_stack.empty() && !prev_backslash)
            return line.substr(0, i);

      }
      prev_backslash = c == '\\' && !prev_backslash;
    }
    return line;
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

  int num_if_nest_;
  stack<IfState*> if_stack_;

  Loc loc_;
  bool fixed_lineno_;

  static DirectiveMap* make_directives_;
  static DirectiveMap* else_if_directives_;
  static size_t shortest_directive_len_;
  static size_t longest_directive_len_;
};

void Parse(Makefile* mk) {
  Parser parser(StringPiece(mk->buf(), mk->len()),
                mk->filename().c_str(),
                mk->mutable_asts());
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
size_t Parser::shortest_directive_len_;
size_t Parser::longest_directive_len_;
