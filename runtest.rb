#!/usr/bin/env ruby
# coding: binary
#
# Copyright 2015 Google Inc. All rights reserved
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

require 'fileutils'

# suppress GNU make jobserver magic when calling "make"
ENV.delete('MAKEFLAGS')
ENV.delete('MAKELEVEL')

while true
  if ARGV[0] == '-s'
    test_serialization = true
    ARGV.shift
  elsif ARGV[0] == '-c'
    ckati = true
    ARGV.shift
    ENV['KATI_VARIANT'] = 'c'
  elsif ARGV[0] == '-n'
    via_ninja = true
    ARGV.shift
    ENV['NINJA_STATUS'] = 'NINJACMD: '
  elsif ARGV[0] == '-a'
    gen_all_targets = true
    ARGV.shift
  elsif ARGV[0] == '-v'
    show_failing = true
    ARGV.shift
  else
    break
  end
end

def get_output_filenames
  files = Dir.glob('*')
  files.delete('Makefile')
  files.delete('build.ninja')
  files.delete('env.sh')
  files.delete('ninja.sh')
  files.delete('gmon.out')
  files.delete('submake')
  files.reject!{|f|f =~ /\.json$/}
  files.reject!{|f|f =~ /^kati\.*/}
  files
end

def cleanup
  (get_output_filenames + Dir.glob('.*')).each do |fname|
    next if fname == '.' || fname == '..'
    FileUtils.rm_rf fname
  end
end

def move_circular_dep(l)
  # We don't care when circular dependency detection happens.
  circ = ''
  while l.sub!(/Circular .* dropped\.\n/, '') do
    circ += $&
  end
  circ + l
end

expected_failures = []
unexpected_passes = []
failures = []
passes = []

if !ARGV.empty?
  test_files = ARGV.map do |test|
    "testcase/#{File.basename(test)}"
  end
else
  test_files = Dir.glob('testcase/*.mk').sort
  test_files += Dir.glob('testcase/*.sh').sort
end

def run_in_testdir(test_filename)
  c = File.read(test_filename)
  name = File.basename(test_filename)
  dir = "out/#{name}"

  FileUtils.mkdir_p(dir)
  Dir.glob("#{dir}/*").each do |fname|
    FileUtils.rm_rf(fname)
  end

  Dir.chdir(dir) do
    yield name
  end
end

def normalize_ninja_log(log, mk)
  log.gsub!(/^NINJACMD: .*\n/, '')
  log.gsub!(/^ninja: no work to do\.\n/, '')
  log.gsub!(/^ninja: error: (.*, needed by .*),.*/,
            '*** No rule to make target \\1.')
  log.gsub!(/^ninja: warning: multiple rules generate (.*)\. builds involving this target will not be correct.*$/,
            'ninja: warning: multiple rules generate \\1.')

  if mk =~ /err_error_in_recipe.mk/
    # This test expects ninja fails. Strip ninja specific error logs.
    ninja_failed_subst = ''
  elsif mk =~ /\/fail_/
    # Recipes in these tests fail.
    ninja_failed_subst = "*** [test] Error 1\n"
  end
  if ninja_failed_subst
    log.gsub!(/^FAILED: (.*\n\/bin\/bash)?.*\n/, ninja_failed_subst)
    log.gsub!(/^ninja: .*\n/, '')
  end
  log
end

def normalize_quotes(log)
  log.gsub!(/[`'"]/, '"')
  # For recent GNU find, which uses Unicode characters.
  log.gsub!(/(\xe2\x80\x98|\xe2\x80\x99)/, '"')
  log
end

def normalize_make_log(expected, mk, via_ninja)
  expected = normalize_quotes(expected)
  expected.gsub!(/^make(?:\[\d+\])?: (Entering|Leaving) directory.*\n/, '')
  expected.gsub!(/^make(?:\[\d+\])?: /, '')
  expected = move_circular_dep(expected)

  # Normalizations for old/new GNU make.
  expected.gsub!(' recipe for target ', ' commands for target ')
  expected.gsub!(' recipe commences ', ' commands commence ')
  expected.gsub!('missing rule before recipe.', 'missing rule before commands.')
  expected.gsub!(' (did you mean TAB instead of 8 spaces?)', '')
  expected.gsub!('Extraneous text after', 'extraneous text after')
  # Not sure if this is useful.
  expected.gsub!(/\s+Stop\.$/, '')
  # GNU make 4.0 has this output.
  expected.gsub!(/Makefile:\d+: commands for target ".*?" failed\n/, '')
  # We treat some warnings as errors.
  expected.gsub!(/^\/bin\/(ba)?sh: line 0: /, '')
  # We print out some ninja warnings in some tests to match what we expect
  # ninja to produce. Remove them if we're not testing ninja.
  if !via_ninja
    expected.gsub!(/^ninja: warning: .*\n/, '')
  end
  # Normalization for "include foo" with C++ kati.
  expected.gsub!(/(: )(\S+): (No such file or directory)\n\*\*\* No rule to make target "\2"./, '\1\2: \3')

  expected
end

def normalize_kati_log(output)
  output = normalize_quotes(output)
  output = move_circular_dep(output)

  # kati specific log messages.
  output.gsub!(/^\*kati\*.*\n/, '')
  output.gsub!(/^c?kati: /, '')
  output.gsub!(/\/bin\/sh: ([^:]*): command not found/,
               "\\1: Command not found")
  output.gsub!(/.*: warning for parse error in an unevaluated line: .*\n/, '')
  output.gsub!(/^FindEmulator: /, '')
  output.gsub!(/^\/bin\/sh: line 0: /, '')
  output.gsub!(/ (\.\/+)+kati\.\S+/, '') # kati log files in find_command.mk
  output.gsub!(/ (\.\/+)+test\S+.json/, '') # json files in find_command.mk
  # Normalization for "include foo" with Go kati.
  output.gsub!(/(: )open (\S+): n(o such file or directory)\nNOTE:.*/,
               "\\1\\2: N\\3")
  output
end

bash_var = ' SHELL=/bin/bash'

run_make_test = proc do |mk|
  c = File.read(mk)
  expected_failure = false
  if c =~ /\A# TODO(?:\(([-a-z|]+)\))?/
    if $1
      todos = $1.split('|')
      if todos.include?('go') && !ckati
        expected_failure = true
      end
      if todos.include?('c') && ckati
        expected_failure = true
      end
      if todos.include?('go-ninja') && !ckati && via_ninja
        expected_failure = true
      end
      if todos.include?('c-ninja') && ckati && via_ninja
        expected_failure = true
      end
      if todos.include?('c-exec') && ckati && !via_ninja
        expected_failure = true
      end
      if todos.include?('ninja') && via_ninja
        expected_failure = true
      end
    else
      expected_failure = true
    end
  end

  run_in_testdir(mk) do |name|
    File.open("Makefile", 'w') do |ofile|
      ofile.print(c)
    end
    File.symlink('../../testcase/submake', 'submake')

    expected = ''
    output = ''

    testcases = c.scan(/^test\d*/).sort.uniq
    if testcases.empty?
      testcases = ['']
    end

    is_silent_test = mk =~ /\/submake_/

    cleanup
    testcases.each do |tc|
      cmd = 'make'
      if via_ninja || is_silent_test
        cmd += ' -s'
      end
      cmd += bash_var
      cmd += " #{tc} 2>&1"
      res = IO.popen(cmd, 'r:binary', &:read)
      res = normalize_make_log(res, mk, via_ninja)
      expected += "=== #{tc} ===\n" + res
      expected_files = get_output_filenames
      expected += "\n=== FILES ===\n#{expected_files * "\n"}\n"
    end

    cleanup
    testcases.each do |tc|
      json = "#{tc.empty? ? 'test' : tc}"
      cmd = "../../kati -save_json=#{json}.json -log_dir=. --use_find_emulator"
      if ckati
        cmd = "../../ckati --use_find_emulator"
      end
      if via_ninja
        cmd += ' --ninja'
      end
      if gen_all_targets
        if !ckati || !via_ninja
          raise "-a should be used with -c -n"
        end
        cmd += ' --gen_all_targets'
      end
      if is_silent_test
        cmd += ' -s'
      end
      cmd += bash_var
      if !gen_all_targets || mk =~ /makecmdgoals/
        cmd += " #{tc}"
      end
      cmd += " 2>&1"
      res = IO.popen(cmd, 'r:binary', &:read)
      if via_ninja && File.exist?('build.ninja') && File.exists?('ninja.sh')
        cmd = './ninja.sh -j1 -v'
        if gen_all_targets
          cmd += " #{tc}"
        end
        cmd += ' 2>&1'
        log = IO.popen(cmd, 'r:binary', &:read)
        res += normalize_ninja_log(log, mk)
      end
      res = normalize_kati_log(res)
      output += "=== #{tc} ===\n" + res
      output_files = get_output_filenames
      output += "\n=== FILES ===\n#{output_files * "\n"}\n"
    end

    File.open('out.make', 'w'){|ofile|ofile.print(expected)}
    File.open('out.kati', 'w'){|ofile|ofile.print(output)}

    if expected =~ /FAIL/
      puts %Q(#{name} has a string "FAIL" in its expectation)
      exit 1
    end

    if expected != output
      if expected_failure
        puts "#{name}: FAIL (expected)"
        expected_failures << name
      else
        puts "#{name}: FAIL"
        failures << name
      end
      if !expected_failure || show_failing
        puts `diff -u out.make out.kati`
      end
    else
      if expected_failure
        puts "#{name}: PASS (unexpected)"
        unexpected_passes << name
      else
        puts "#{name}: PASS"
        passes << name
      end
    end

    if name !~ /^err_/ && test_serialization && !expected_failure
      testcases.each do |tc|
        json = "#{tc.empty? ? 'test' : tc}"
        cmd = "../../kati -save_json=#{json}_2.json -load_json=#{json}.json -n -log_dir=. #{tc} 2>&1"
        res = IO.popen(cmd, 'r:binary', &:read)
        if !File.exist?("#{json}.json") || !File.exist?("#{json}_2.json")
          puts "#{name}##{json}: Serialize failure (not exist)"
          puts res
        else
          json1 = File.read("#{json}.json")
          json2 = File.read("#{json}_2.json")
          if json1 != json2
            puts "#{name}##{json}: Serialize failure"
            puts res
          end
        end
      end
    end
  end
end

run_shell_test = proc do |sh|
  is_ninja_test = sh =~ /\/ninja_/
  if is_ninja_test && (!ckati || !via_ninja)
    next
  end

  run_in_testdir(sh) do |name|
    cleanup
    cmd = "sh ../../#{sh} make"
    if is_ninja_test
      cmd += ' -s'
    end
    cmd += bash_var
    expected = IO.popen(cmd, 'r:binary', &:read)
    cleanup

    if is_ninja_test
      if ckati
        cmd = "sh ../../#{sh} ../../ckati --ninja --regen"
      else
        next
      end
    else
      if ckati
        cmd = "sh ../../#{sh} ../../ckati"
      else
        cmd = "sh ../../#{sh} ../../kati --use_cache -log_dir=."
      end
    end
    cmd += bash_var

    output = IO.popen(cmd, 'r:binary', &:read)

    expected = normalize_make_log(expected, sh, is_ninja_test)
    output = normalize_kati_log(output)
    if is_ninja_test
      output = normalize_ninja_log(output, sh)
    end
    File.open('out.make', 'w'){|ofile|ofile.print(expected)}
    File.open('out.kati', 'w'){|ofile|ofile.print(output)}

    if expected != output
      puts "#{name}: FAIL"
      puts `diff -u out.make out.kati`
      failures << name
    else
      puts "#{name}: PASS"
      passes << name
    end
  end
end

test_files.each do |test|
  if /\.mk$/ =~ test
    run_make_test.call(test)
  elsif /\.sh$/ =~ test
    run_shell_test.call(test)
  else
    raise "Unknown test type: #{test}"
  end
end

puts

if !expected_failures.empty?
  puts "=== Expected failures ==="
  expected_failures.each do |n|
    puts n
  end
end

if !unexpected_passes.empty?
  puts "=== Unexpected passes ==="
  unexpected_passes.each do |n|
    puts n
  end
end

if !failures.empty?
  puts "=== Failures ==="
  failures.each do |n|
    puts n
  end
end

puts

if !unexpected_passes.empty? || !failures.empty?
  puts "FAIL! (#{failures.size + unexpected_passes.size} fails #{passes.size} passes)"
  exit 1
else
  puts 'PASS!'
end
