#!/usr/bin/python
#
# Copyright 2016 Google Inc. All rights reserved
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

import atexit
import os
import re
import subprocess
import sys
import tempfile


UNKNOWN_TYPE = -1
READ_CONTENT = 0
WRITE_CONTENT = 1
REMOVE_CONTENT = 2
READ_METADATA = 3
WRITE_METADATA = 4
FORK_PROCESS = 5
CLONE_THREAD = 6
CHANGE_DIR = 7
CHANGE_ROOT = 8
CREATE_LINK = 9
RENAME_FILE = 10
GET_CWD = 11
DUP_FD = 12
CHANGE_DIR_FD = 13

SYSCALLS = {
  'access': (READ_METADATA, False, True),
  'acct': (WRITE_METADATA, False, True),
  'chdir': (CHANGE_DIR, False, True),
  'chmod': (WRITE_METADATA, False, True),
  'chown': (WRITE_METADATA, False, True),
  'chroot': (CHANGE_ROOT, False, True),
  'clone': (UNKNOWN_TYPE, False, False),
  'creat': (WRITE_CONTENT, False, True),
  'execve': (READ_CONTENT, False, True),
  'faccessat': (READ_METADATA, True, True),
  'fchmodat': (WRITE_METADATA, True, True),
  'fchownat': (WRITE_METADATA, True, True),
  'fork': (FORK_PROCESS, False, False),
  'fstatat': (READ_METADATA, True, True),
  'futimesat': (WRITE_METADATA, True, True),
  'getcwd': (GET_CWD, False, False),
  'lchown': (WRITE_METADATA, False, True),
  'link': (CREATE_LINK, False, True),
  'linkat': (CREATE_LINK, True, True),
  'lstat': (READ_METADATA, False, True),
  'mkdir': (WRITE_CONTENT, False, True),
  'mkdirat': (WRITE_CONTENT, True, True),
  'mknod': (WRITE_METADATA, False, True),
  'mknodat': (WRITE_METADATA, True, True),
  'newfstatat': (READ_METADATA, True, True),
  'open': (UNKNOWN_TYPE, False, True),
  'openat': (UNKNOWN_TYPE, True, True),
  'readlink': (READ_METADATA, False, True),
  'readlinkat': (READ_METADATA, True, True),
  'rename': (RENAME_FILE, False, True),
  'renameat': (RENAME_FILE, True, True),
  'rmdir': (REMOVE_CONTENT, False, True),
  'stat': (READ_METADATA, False, True),
  'stat64': (READ_METADATA, False, True),
  'statfs': (READ_METADATA, False, True),
  'symlink': (CREATE_LINK, False, True),
  'symlinkat': (CREATE_LINK, True, True),
  'truncate': (WRITE_CONTENT, False, True),
  'unlink': (REMOVE_CONTENT, False, True),
  'unlinkat': (REMOVE_CONTENT, True, True),
  'uselib': (UNKNOWN_TYPE, False, True),
  'utime': (WRITE_METADATA, False, True),
  'utimensat': (WRITE_METADATA, True, True),
  'utimes': (WRITE_METADATA, False, True),
  'vfork': (FORK_PROCESS, False, False),
  'dup': (DUP_FD, False, False),
  'dup2': (DUP_FD, False, False),
  'dup3': (DUP_FD, False, False),
  'fcntl': (DUP_FD, False, False),
  'fchdir': (CHANGE_DIR_FD, False, False),
}


class ProcessState(object):
  def __init__(self, cwd):
    self.unfinished = {}
    self.cwd = cwd
    self.fds = {}

  def set_unfinished(self, tid, info):
    assert tid not in self.unfinished
    self.unfinished[tid] = info

  def get_unfinished(self, tid):
    assert tid in self.unfinished
    return self.unfinished.pop(tid)

  def abspath(self, path):
    return os.path.abspath(os.path.join(self.cwd, path))

  def chdir(self, path):
    self.cwd = os.path.join(self.cwd, path)

  def resolve_at(self, fd, path):
    if 'AT_FDCWD' in fd:
      return path
    fd = int(fd)
    return os.path.abspath(os.path.join(self.fds[fd], path))

  def opendir(self, fd, path):
    self.fds[fd] = path

  def dup(self, oldfd, newfd):
    if oldfd in self.fds:
      self.fds[newfd] = self.fds[oldfd]


class StraceLogTracker(object):
  def __init__(self):
    self.inputs = set()
    self.outputs = set()
    self.procs = {}
    self.root_pid = None
    self.fork_parent = None

  @staticmethod
  def classify_syscall(syscall, args):
    if syscall == 'clone':
      return CLONE_THREAD if 'CLONE_THREAD' in args else FORK_PROCESS
    if syscall == 'open' or syscall == 'openat':
      if 'O_WRONLY' in args or 'O_RDWR' in args:
        return WRITE_CONTENT
      return READ_CONTENT
    if syscall == 'fcntl':
      if 'F_DUPFD' in args:
        return DUP_FD
      return READ_METADATA
    if syscall not in SYSCALLS:
      raise Exception('Unknown syscall: ' + syscall)
    return SYSCALLS[syscall][0]

  def parse_strace_line(self, proc, line):
    m = re.match(r'<\.\.\. (\w+) resumed> .* = (-?\d+|\?)', line)
    if m:
      syscall = m.group(1)
      retval = m.group(2)
      if retval != '?':
        retval = int(retval)
      return syscall, None, retval, None
    m = re.match(r'(\w+)\((.*)', line)
    if not m:
      raise Exception('Unexpected line: ' + line)
    syscall = m.group(1)
    rest = m.group(2)

    typ = self.classify_syscall(syscall, rest)

    # TODO: Improve the argument parser.
    paths = []
    for path in re.findall(r'".*?"', rest):
      paths.append(path[1:-1])

    # This is xxxat.
    if SYSCALLS[syscall][1]:
      fd_end = rest.index(',')
      fd = rest[:fd_end]
      paths[0] = proc.resolve_at(fd, paths[0])
      if syscall == 'symlinkat' or syscall == 'renameat':
        r = rest[fd_end:]
        r = re.sub(r'".*", ', '', r, 1)
        fd_end = r.index(',')
        fd = r[:fd_end]
        paths[1] = proc.resolve_at(fd, paths[1])

    if typ == CREATE_LINK:
      newpath = proc.abspath(paths[1])
      oldpath = os.path.join(os.path.dirname(newpath), paths[0])
      paths[0] = oldpath
      paths[1] = newpath
    else:
      paths = map(proc.abspath, paths)

    if typ == DUP_FD or typ == CHANGE_DIR_FD:
      m = re.match(r'\d+', rest)
      if m:
        assert not paths
        # TODO: Stop abusing paths for FD.
        paths = int(m.group(0))

    retval = None
    if not rest.endswith(' <unfinished ...>'):
      m = re.search(r' = (-?\d+)', rest)
      if not m:
        raise Exception('Unexpected line: ' + line)
      retval = int(m.group(1))
    return syscall, paths, retval, typ

  def handle_strace_line(self, line):
    pid, line = line.split(' ', 1)
    pid = int(pid)
    line = line.strip()

    if not self.root_pid:
      self.root_pid = pid
      self.procs[pid] = ProcessState(os.getcwd())

    if line.startswith('+++ '):
      assert pid in self.procs
      self.procs[pid] = None
      return
    if line.startswith('--- '):
      return

    # TODO: Not sure what this is.
    if line.startswith('utimensat(0, NULL, NULL, 0)'):
      return

    # A bug of strace?
    if line.startswith('<... futex resumed>'):
      return

    if pid not in self.procs:
      # TODO: fix
      #assert self.fork_parent
      if self.fork_parent:
        self.procs[pid] = ProcessState(self.fork_parent.cwd)
        self.fork_parent = None
      else:
        self.procs[pid] = ProcessState(self.procs[self.root_pid].cwd)

    assert pid in self.procs
    proc = self.procs[pid]
    syscall, paths, retval, typ = self.parse_strace_line(proc, line)
    #print (pid, syscall, paths, retval, typ)

    if retval is None:
      #if typ == FORK_PROCESS:
      if syscall == 'vfork':
        # This probably means vfork. This assert may fail when
        # multiple processes vfork at once.
        # TODO: Come up with a better way.
        assert not self.fork_parent
        self.fork_parent = proc
      proc.set_unfinished(pid, (syscall, paths, typ))
    else:
      if typ is None:
        sc, paths, typ = proc.get_unfinished(pid)
        assert sc == syscall

      if typ == UNKNOWN_TYPE:
        raise Exception('Unknown syscall: ' + line)

      if syscall == 'open' or syscall == 'openat':
        if retval >= 0:
          proc.opendir(retval, paths[0])

      if typ in [GET_CWD, READ_METADATA, WRITE_METADATA]:
        return

      if typ == CLONE_THREAD:
        assert retval not in self.procs
        self.procs[retval] = proc
        return

      if typ == FORK_PROCESS:
        if retval not in self.procs:
          self.procs[retval] = ProcessState(proc.cwd)
        self.fork_parent = None
        return

      if typ == CHANGE_DIR:
        proc.chdir(paths[0])
        return

      if typ == CHANGE_DIR_FD:
        assert paths
        d = proc.resolve_at(str(paths), '.')
        proc.chdir(d)
        return

      if typ == READ_CONTENT:
        if retval >= 0 and paths[0] not in self.outputs:
          self.inputs.add(paths[0])
        return

      if typ == WRITE_CONTENT:
        if retval >= 0 and paths[0] not in self.inputs:
          self.outputs.add(paths[0])
        return

      if typ == REMOVE_CONTENT:
        if retval >= 0:
          self.outputs.discard(paths[0])
        return

      if typ == RENAME_FILE:
        if retval >= 0:
          self.outputs.discard(paths[0])
          if paths[1] not in self.inputs:
            self.outputs.add(paths[1])
        return

      if typ == CREATE_LINK:
        if retval >= 0:
          if paths[0] not in self.outputs:
            self.inputs.add(paths[0])
          if paths[1] not in self.inputs:
            self.outputs.add(paths[1])
        return

      if typ == DUP_FD:
        assert paths
        if retval >= 0:
          proc.dup(paths, retval)
        return

      assert False


if len(sys.argv) <= 2:
  print('Usage: %s outfile command [args]' % sys.argv[0])
  sys.exit(1)

timed_out = False
outfile = sys.argv[1]
if outfile == '-f':
  filename = sys.argv[2]
  outfile = None
  status = 0
else:
  fd, filename = tempfile.mkstemp(prefix='dsan')
  atexit.register(lambda: os.remove(filename))
  os.close(fd)

  strace_args = (
      ['timeout',
       '-k', '30', '180',  # TERM after 3 mins then KILL after 30 secs
       'strace',
       '-f',  # follow child
       '-e', 'trace=clone,fork,vfork,file,dup,dup2,dup3,fcntl,fchdir',
       '-o' + filename] + sys.argv[2:])

  status = subprocess.call(strace_args)
  if status == 124 or status == 128 + 9 or status == -9:
    timed_out = True
    status = subprocess.call(sys.argv[2:])

  raw_log = None
  raw_log = open(outfile + '.raw', 'w')
  if raw_log:
    with open(filename) as f:
      for line in f:
        raw_log.write(line)
  raw_log.close()

tracker = StraceLogTracker()
with open(filename) as f:
  for line in f:
    #print line
    try:
      tracker.handle_strace_line(line)
    except:
      sys.stderr.write('EXCEPTION: ' + line)
      raise

if outfile:
  f = open(outfile, 'w')
else:
  f = sys.stdout

if timed_out:
  f.write('TIMED OUT\n')
for path in tracker.inputs:
  f.write(path + '\n')
f.write('\n')
for path in tracker.outputs:
  f.write(path + '\n')

sys.exit(status)
