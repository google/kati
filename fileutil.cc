#include "fileutil.h"

#include <errno.h>
#include <limits.h>
#include <sys/stat.h>
#include <unistd.h>

#include "log.h"

bool Exists(StringPiece filename) {
  CHECK(filename.size() < PATH_MAX);
  char buf[PATH_MAX+1];
  memcpy(buf, filename.data(), filename.size());
  buf[filename.size()] = 0;
  struct stat st;
  if (stat(buf, &st) < 0) {
    return false;
  }
  return true;
}
