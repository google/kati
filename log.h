#ifndef LOG_H_
#define LOG_H_

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "stringprintf.h"

#define LOG(args...) do {                                           \
    fprintf(stderr, "*kati*: %s\n", StringPrintf(args).c_str());    \
  } while(0)

#define PERROR(...) do {                                            \
    fprintf(stderr, "%s: %s\n", StringPrintf(__VA_ARGS__).c_str(),  \
            strerror(errno));                                       \
    exit(1);                                                        \
  } while (0)

#define WARN(...) do {                                          \
    fprintf(stderr, "%s\n", StringPrintf(__VA_ARGS__).c_str()); \
  } while (0)

#define ERROR(...) do {                                         \
    fprintf(stderr, "%s\n", StringPrintf(__VA_ARGS__).c_str()); \
    exit(1);                                                    \
  } while (0)

#define CHECK(c) if (!(c)) ERROR("%s:%d: %s", __FILE__, __LINE__, #c)

#endif  // LOG_H_
