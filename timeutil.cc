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

#include "timeutil.h"

#include <sys/time.h>
#include <time.h>

#include "log.h"

double GetTime() {
#if defined(__linux__)
  struct timespec ts;
  clock_gettime(CLOCK_REALTIME, &ts);
  return ts.tv_sec + ts.tv_nsec * 0.001 * 0.001 * 0.001;
#else
  struct timeval tv;
  if (gettimeofday(&tv, NULL) < 0)
    PERROR("gettimeofday");
  return tv.tv_sec + tv.tv_usec * 0.001 * 0.001;
#endif
}

ScopedTimeReporter::ScopedTimeReporter(const char* name)
    : name_(name), start_(GetTime()) {}

ScopedTimeReporter::~ScopedTimeReporter() {
  double elapsed = GetTime() - start_;
  LOG_STAT("%s: %f", name_, elapsed);
}
