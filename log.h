#ifndef LOG_H_
#define LOG_H_

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define LOG(fmt, ...) do {                                              \
    char buf[999];                                                      \
    sprintf(buf, fmt, __VA_ARGS__);                                     \
    fprintf(stderr, "*kati*: %s\n", buf);                               \
  } while(0)

#define PERROR(...) do {                                        \
    char buf[999];                                              \
    sprintf(buf, __VA_ARGS__);                                  \
    fprintf(stderr, "%s: %s\n", buf, strerror(errno));          \
    exit(1);                                                    \
  } while (0)

#define ERROR(...) do {                                         \
    char buf[999];                                              \
    sprintf(buf, __VA_ARGS__);                                  \
    fprintf(stderr, "%s\n", buf);                               \
    exit(1);                                                    \
  } while (0)

#define CHECK(c) if (!(c)) ERROR("%s:%d: %s", __FILE__, __LINE__, #c)

#endif  // LOG_H_
