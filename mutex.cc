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

#include "mutex.h"

#include "log.h"

Mutex::Mutex() {
  if (pthread_mutex_init(&mu_, NULL) != 0)
    PERROR("pthread_mutex_init");
}

Mutex::~Mutex() {
  if (pthread_mutex_destroy(&mu_) != 0)
    PERROR("pthread_mutex_destroy");
}

void Mutex::lock() {
  if (pthread_mutex_lock(&mu_) != 0)
    PERROR("pthread_mutex_lock");
}

void Mutex::unlock() {
  if (pthread_mutex_unlock(&mu_) != 0)
    PERROR("pthread_mutex_unlock");
}
