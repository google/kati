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
import errno
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

EXITED = 20
SIGNALED = 21
DONT_CARE = 22

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
    self.cwd = cwd
    self.fds = {}

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


class StraceEvent(object):
  def __init__(self, line):
    pid, line = line.split(' ', 1)
    pid = int(pid)
    line = line.strip()
    self.pid = pid
    self.line = line
    self.syscall = None
    self.paths = None
    self.retval = None
    self.typ = UNKNOWN_TYPE
    self.fd = None
    self.is_resumed = False

    if line.startswith('+++ '):
      self.typ = EXITED
      return

    if line.startswith('--- '):
      self.typ = SIGNALED
      return

    self.parse_strace_line(line)

  def __str__(self):
    return '%d %s' % (self.pid, self.str_helper())

  def str_helper(self):
    if self.typ == UNKNOWN_TYPE:
      return 'UNKNOWN (%s)' % self.line
    if self.typ == EXITED:
      return 'EXITED (%s)' % self.line
    if self.typ == SIGNALED:
      return 'SIGNALED (%s)' % self.line
    if self.typ == DONT_CARE:
      return 'DONT_CARE (%s)' % self.line

    paths = self.paths
    if not self.paths:
      paths = []
      if self.fd is not None:
        paths.append(self.fd)

    subtype = ''
    if self.syscall == 'clone':
      subtype = '(fork)' if self.typ == FORK_PROCESS else '(thread)'
    if self.syscall == 'open' or self.syscall == 'openat':
      subtype = '(write)' if self.typ == WRITE_CONTENT else '(read)'
    return '%s%s %s = %s' % (self.syscall, subtype, paths, self.retval)

  def is_syscall(self):
    return self.typ != UNKNOWN_TYPE and self.typ < EXITED

  def is_finished(self):
    return not self.is_syscall() or self.retval is not None

  def merge(self, e):
    assert not self.is_resumed
    assert e.is_resumed
    assert self.pid == e.pid
    assert self.syscall == e.syscall
    assert e.paths is None
    assert e.fd is None
    assert not self.retval
    self.retval = e.retval
    if self.typ == UNKNOWN_TYPE:
      self.typ = e.typ

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

  def parse_strace_line(self, line):
    m = re.match(r'<\.\.\. (\w+) resumed> (.*) = (-?\d+|\?)', line)
    if m:
      self.syscall = m.group(1)
      self.typ = self.classify_syscall(self.syscall, m.group(2))
      self.retval = m.group(3)
      self.is_resumed = True
      if self.retval != '?':
        self.retval = int(self.retval)
      return
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

    if typ == DUP_FD or typ == CHANGE_DIR_FD:
      m = re.match(r'\d+', rest)
      assert m
      assert not paths
      self.fd = int(m.group(0))

    retval = None
    if not rest.endswith(' <unfinished ...>'):
      m = re.search(r' = (-?\d+)', rest)
      if not m:
        raise Exception('Unexpected line: ' + line)
      retval = int(m.group(1))
    self.syscall = syscall
    self.paths = paths
    self.retval = retval
    self.typ = typ
    self.rest = rest

  def resolve_paths(self, proc):
    # This is xxxat.
    if SYSCALLS[self.syscall][1]:
      rest = self.rest
      fd_end = rest.index(',')
      fd = rest[:fd_end]
      self.paths[0] = proc.resolve_at(fd, self.paths[0])
      if self.syscall == 'symlinkat' or self.syscall == 'renameat':
        r = rest[fd_end:]
        r = re.sub(r'".*", ', '', r, 1)
        fd_end = r.index(',')
        fd = r[:fd_end]
        self.paths[1] = proc.resolve_at(fd, self.paths[1])

    if self.typ == CREATE_LINK:
      newpath = proc.abspath(self.paths[1])
      oldpath = os.path.join(os.path.dirname(newpath), self.paths[0])
      self.paths[0] = oldpath
      self.paths[1] = newpath
    elif self.paths:
      self.paths = map(proc.abspath, self.paths)


class StraceLogTracker(object):
  def __init__(self, filename):
    self.inputs = set()
    self.outputs = set()
    self.procs = {}
    self.root_pid = None
    self.filename = filename

    events = self.read_strace_log(filename)
    self.handle_strace_events(events)

  def read_strace_log(self, filename):
    events = []
    # A map from a PID to an unfinished event.
    unfinished_events = {}
    with open(filename) as f:
      for line in f:
        event = StraceEvent(line)
        prev_event = None
        if event.pid in unfinished_events:
          prev_event = unfinished_events.pop(event.pid)

        if event.is_finished():
          if event.is_resumed:
            assert prev_event
            prev_event.merge(event)
          else:
            assert not prev_event
            events.append(event)
        else:
          assert not prev_event
          unfinished_events[event.pid] = event
          events.append(event)
    assert not unfinished_events
    #for event in events: print str(event)
    return events

  def handle_strace_events(self, events):
    for event in events:
      #print str(event)
      try:
        self.handle_strace_event(event)
      except:
        sys.stderr.write('EXCEPTION: %s in %s\n' % (str(event), self.filename))
        raise

  def handle_strace_event(self, event):
    pid = event.pid
    syscall = event.syscall
    retval = event.retval
    typ = event.typ

    if not self.root_pid:
      self.root_pid = pid
      self.procs[pid] = ProcessState(os.getcwd())
    proc = self.procs[pid]

    if typ == EXITED:
      assert pid in self.procs
      self.procs[pid] = None
      return
    if typ == SIGNALED:
      return
    if typ == DONT_CARE:
      return
    if typ == UNKNOWN_TYPE:
      raise Exception('Unknown syscall: ' + event.line)
    if typ in [GET_CWD, READ_METADATA, WRITE_METADATA]:
      return

    assert pid in self.procs
    assert retval is not None
    assert event.is_syscall()

    event.resolve_paths(proc)
    paths = event.paths

    if typ == FORK_PROCESS:
      if retval > 0:
        self.procs[retval] = ProcessState(proc.cwd)
      return

    if syscall == 'open' or syscall == 'openat':
      if retval >= 0:
        proc.opendir(retval, paths[0])

    if typ == CLONE_THREAD:
      assert retval not in self.procs
      self.procs[retval] = proc
      return

    if typ == CHANGE_DIR:
      proc.chdir(paths[0])
      return

    if typ == CHANGE_DIR_FD:
      assert event.fd is not None
      d = proc.resolve_at(str(event.fd), '.')
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
        self.inputs.discard(paths[0])
        self.outputs.discard(paths[0])
      return

    if typ == RENAME_FILE:
      if retval >= 0:
        self.inputs.discard(paths[0])
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
      assert event.fd is not None
      if retval >= 0:
        proc.dup(event.fd, retval)
      return

    assert False


if len(sys.argv) <= 2:
  print('Usage: %s outfile command [args]' % sys.argv[0])
  sys.exit(1)

timed_out = False
outfile = sys.argv[1]
try:
  os.makedirs(os.path.dirname(outfile))
except OSError as e:
  if e.errno != errno.EEXIST:
    raise

if outfile == '-f':
  filename = sys.argv[2]
  outfile = None
  status = 0
else:
  fd, filename = tempfile.mkstemp(prefix='dsan')
  atexit.register(lambda: os.remove(filename))
  os.close(fd)

  strace_args = (
      [os.path.join(os.path.dirname(__file__), 'strace/strace'),
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

tracker = StraceLogTracker(filename)

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
