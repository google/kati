#!/usr/bin/env python
#
# Copyright (C) 2009 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

#
# Finds files with the specified name under a particular directory, stopping
# the search in a given subdirectory when the file is found.
#

import os
import sys

def perform_find(mindepth, prune, dirlist, filenames):
  result = []
  pruneleaves = set(map(lambda x: os.path.split(x)[1], prune))
  for rootdir in dirlist:
    rootdepth = rootdir.count("/")
    for root, dirs, files in os.walk(rootdir, followlinks=True):
      # prune
      check_prune = False
      for d in dirs:
        if d in pruneleaves:
          check_prune = True
          break
      if check_prune:
        i = 0
        while i < len(dirs):
          if dirs[i] in prune:
            del dirs[i]
          else:
            i += 1
      # mindepth
      if mindepth > 0:
        depth = 1 + root.count("/") - rootdepth
        if depth < mindepth:
          continue
      # match
      for filename in filenames:
        if filename in files:
          result.append(os.path.join(root, filename))
          del dirs[:]
  return result

def usage():
  sys.stderr.write("""Usage: %(progName)s [<options>] [--dir=<dir>] <filenames>
Options:
   --mindepth=<mindepth>
       Both behave in the same way as their find(1) equivalents.
   --prune=<dirname>
       Avoids returning results from inside any directory called <dirname>
       (e.g., "*/out/*"). May be used multiple times.
   --dir=<dir>
       Add a directory to search.  May be repeated multiple times.  For backwards
       compatibility, if no --dir argument is provided then all but the last entry
       in <filenames> are treated as directories.
""" % {
      "progName": os.path.split(sys.argv[0])[1],
    })
  sys.exit(1)

def main(argv):
  mindepth = -1
  prune = []
  dirlist = []
  i=1
  while i<len(argv) and len(argv[i])>2 and argv[i][0:2] == "--":
    arg = argv[i]
    if arg.startswith("--mindepth="):
      try:
        mindepth = int(arg[len("--mindepth="):])
      except ValueError:
        usage()
    elif arg.startswith("--prune="):
      p = arg[len("--prune="):]
      if len(p) == 0:
        usage()
      prune.append(p)
    elif arg.startswith("--dir="):
      d = arg[len("--dir="):]
      if len(p) == 0:
        usage()
      dirlist.append(d)
    else:
      usage()
    i += 1
  if len(dirlist) == 0: # backwards compatibility
    if len(argv)-i < 2: # need both <dirlist> and <filename>
      usage()
    dirlist = argv[i:-1]
    filenames = [argv[-1]]
  else:
    if len(argv)-i < 1: # need <filename>
      usage()
    filenames = argv[i:]
  results = list(set(perform_find(mindepth, prune, dirlist, filenames)))
  results.sort()
  for r in results:
    print r

if __name__ == "__main__":
  main(sys.argv)
