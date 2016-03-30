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

import os
import sys


WHITELIST = [
    # This is created at ckati-time.
    'out/build_number.txt',
    'out/build_date.txt',
    # This wouldn't be updated often.
    # TODO: That said, it'd be better to have this in the dependencies.
    'build/tools/normalize_path.py',
]
WHITELIST = set(os.path.join(os.getcwd(), p) for p in WHITELIST)

OUTPUT_WHITELIST = [
    'out/host/common/obj/PACKAGING/gpl_source_intermediates/gpl_source.tgz',
]
OUTPUT_WHITELIST = set(os.path.join(os.getcwd(), p) for p in OUTPUT_WHITELIST)

class DepSanitizer(object):
  def __init__(self, dsan_dir):
    self.dsan_dir = dsan_dir
    self.outputs = {}
    self.defaults = []
    self.checked = {}
    self.cwd = os.getcwd()
    self.has_error = False

  def add_node(self, output, rule, inputs, depfile):
    assert output not in self.outputs
    self.outputs[output] = (rule, inputs, depfile)

  def set_defaults(self, defaults):
    assert not self.defaults
    self.defaults = defaults

  def run(self):
    for o in self.defaults:
      self.check_dep_rec(o)

  def read_inputs_from_depfile(self, rule, depfile):
    # TODO: Comment out this.
    if not os.path.exists(depfile):
      print '%s: %s file not exists!' % (rule, depfile)
      self.has_error = True
      return []

    r = []
    with open(depfile) as f:
      for tok in f.read().split():
        if tok.endswith(':') or tok == '\\':
          continue
        r.append(tok)
    return r

  def check_dep_rec(self, output):
    if output not in self.outputs:
      # Leaf node.
      return set((output,))

    if output in self.checked:
      return self.checked[output]
    # TODO: Why do we need this?
    self.checked[output] = set()

    rule, inputs, depfile = self.outputs[output]
    if depfile:
      inputs += self.read_inputs_from_depfile(rule, depfile)

    products = set()
    for input in inputs:
      products |= self.check_dep_rec(input)

    actual_outputs = set()
    if rule != 'phony':
      actual_outputs = self.check_dep(output, rule, inputs, products)

    r = products | actual_outputs
    self.checked[output] = r
    return r

  def parse_trace_file(self, err_prefix, fn):
    # TODO: Comment out this.
    if not os.path.exists(fn):
      print '%s: %s file not exists!' % (err_prefix, fn)
      self.has_error = True
      return set(), set()

    actual_inputs = set()
    actual_outputs = set()
    with open(fn) as f:
      a = actual_inputs
      for line in f:
        line = line.strip()
        if line == '':
          a = actual_outputs
        elif line == 'TIMED OUT':
          print '%s: timed out - diagnostics will be incomplete' % err_prefix
        else:
          a.add(line)
    return actual_inputs, actual_outputs

  def check_dep(self, output, rule, inputs, products):
    err_prefix = '%s(%s)' % (rule, output)

    fn = os.path.join(self.dsan_dir, rule + '.trace')
    actual_inputs, actual_outputs = self.parse_trace_file(err_prefix, fn)

    output = os.path.abspath(os.path.join(self.cwd, output))
    inputs = set(os.path.abspath(os.path.join(self.cwd, i)) for i in inputs)

    if output not in actual_outputs:
      print '%s: should not have %s as the output' % (err_prefix, output)
      self.has_error = True

    undefined_inputs = actual_inputs - inputs - products
    if output in OUTPUT_WHITELIST:
      undefined_inputs = set()
    for undefined_input in undefined_inputs:
      if not undefined_input.startswith(self.cwd):
        continue

      # For Android
      # TODO: Move to an external file?
      if undefined_input in WHITELIST:
        continue
      if undefined_input.startswith(os.path.join(self.cwd, 'prebuilts')):
        continue
      # Python automatically creates them.
      if undefined_input.endswith('.pyc'):
        continue
      # Ninja's rspfile.
      if undefined_input == output + '.rsp':
        continue

      if os.path.isdir(undefined_input):
        continue
      print '%s: should have %s as an input' % (err_prefix, undefined_input)
      self.has_error = True

    return actual_outputs


if len(sys.argv) != 3:
  print('Usage: %s dsandir build.ninja' % sys.argv[0])
  sys.exit(1)

dsan = DepSanitizer(sys.argv[1])

depfile = None
sys.stderr.write('Parsing %s...\n' % sys.argv[2])
with open(sys.argv[2]) as f:
  for line in f:
    line = line.rstrip()
    if line.startswith('build '):
      toks = line.split(' ')
      output = toks[1][0:-1]
      rule = toks[2]
      inputs = toks[3:]
      dsan.add_node(output, rule, inputs, depfile)
      depfile = None
    elif line.startswith('default '):
      dsan.set_defaults(line.split(' ')[1:])
    elif line.startswith(' depfile = '):
      assert not depfile
      depfile = line.split(' ')[3]

dsan.run()

if dsan.has_error:
  sys.exit(1)
