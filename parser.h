#ifndef PARSER_H_
#define PARSER_H_

class Makefile;

void Parse(Makefile* mk);

void InitParser();
void QuitParser();

#endif  // PARSER_H_
