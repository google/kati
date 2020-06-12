// Copyright 2016 Google Inc. All rights reserved
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

#include "affinity.h"

#include "flags.h"
#include "log.h"

#ifdef __linux__

#include <sched.h>
#include <sys/types.h>
#include <unistd.h>

#include <random>

void SetAffinityForSingleThread() {
  cpu_set_t cs;
  CPU_ZERO(&cs);
  std::random_device generator;
  std::uniform_int_distribution<int> distribution(0, g_flags.num_cpus - 1);
  int cpu = distribution(generator);

  // Try to come up with a CPU and one close to it. This should work on most
  // hyperthreaded system, but may be less optimal under stranger setups.
  // Choosing two completely different CPUs would work here as well, it's just a
  // couple percent faster if they're close (and still faster than letting the
  // scheduler do whatever it wants).
  cpu = cpu - (cpu % 2);
  CPU_SET(cpu, &cs);
  if (g_flags.num_cpus > 1)
    CPU_SET(cpu + 1, &cs);

  if (sched_setaffinity(0, sizeof(cs), &cs) < 0)
    WARN("sched_setaffinity: %s", strerror(errno));
}

void SetAffinityForMultiThread() {
  cpu_set_t cs;
  CPU_ZERO(&cs);
  for (int i = 0; i < g_flags.num_cpus; i++) {
    CPU_SET(i, &cs);
  }
  if (sched_setaffinity(0, sizeof(cs), &cs) < 0)
    WARN("sched_setaffinity: %s", strerror(errno));
}

#else

void SetAffinityForSingleThread() {}
void SetAffinityForMultiThread() {}

#endif
