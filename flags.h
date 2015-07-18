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

#ifndef FLAGS_H_
#define FLAGS_H_

extern bool g_is_dry_run;
extern bool g_enable_stat_logs;
extern const char* g_ignore_optional_include_pattern;
extern const char* g_goma_dir;
extern int g_num_jobs;
extern bool g_detect_android_echo;
extern bool g_gen_regen_rule;
extern bool g_error_on_env_change;

#endif  // FLAGS_H_
