#ifndef PARSER_H_
#define PARSER_H_

#include <vector>

#include "loc.h"
#include "string_piece.h"

using namespace std;

class AST;
class Makefile;

void Parse(Makefile* mk);
void Parse(StringPiece buf, const Loc& loc, vector<AST*>* out_asts);

void InitParser();
void QuitParser();

#endif  // PARSER_H_
