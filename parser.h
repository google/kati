#ifndef PARSER_H_
#define PARSER_H_

#include <vector>

#include "ast.h"
#include "loc.h"
#include "string_piece.h"

using namespace std;

class Makefile;

void Parse(Makefile* mk);
void Parse(StringPiece buf, const Loc& loc, vector<AST*>* out_asts);

void ParseAssignStatement(StringPiece line, size_t sep,
                          StringPiece* lhs, StringPiece* rhs, AssignOp* op);

void InitParser();
void QuitParser();

#endif  // PARSER_H_
