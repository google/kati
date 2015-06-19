#ifndef LOG_H_
#define LOG_H_

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define LOG(args...) do {                       \
    char log_buf[999];                          \
    sprintf(log_buf, args);                     \
    fprintf(stderr, "*kati*: %s\n", log_buf);   \
  } while(0)

#define PERROR(...) do {                                        \
    char log_buf[999];                                          \
    sprintf(log_buf, __VA_ARGS__);                              \
    fprintf(stderr, "%s: %s\n", log_buf, strerror(errno));      \
    exit(1);                                                    \
  } while (0)

#define WARN(...) do {                          \
    char log_buf[999];                          \
    sprintf(log_buf, __VA_ARGS__);              \
    fprintf(stderr, "%s\n", log_buf);           \
  } while (0)

#define ERROR(...) do {                         \
    char log_buf[999];                          \
    sprintf(log_buf, __VA_ARGS__);              \
    fprintf(stderr, "%s\n", log_buf);           \
    exit(1);                                    \
  } while (0)

#define CHECK(c) if (!(c)) ERROR("%s:%d: %s", __FILE__, __LINE__, #c)

#endif  // LOG_H_
