#!/usr/bin/env ruby

require 'fileutils'

if ARGV[0] == '-s'
  test_serialization = true
end

def get_output_filenames
  files = Dir.glob('*')
  files.delete('Makefile')
  files.reject!{|f|f =~ /\.json$/}
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

def normalize_make_log(expected)
  expected.gsub!(/^make(?:\[\d+\])?: (Entering|Leaving) directory.*\n/, '')
  expected.gsub!(/^make(?:\[\d+\])?: /, '')
  expected = move_circular_dep(expected)

  # Normalizations for old/new GNU make.
  expected.gsub!(/[`'"]/, '"')
  expected.gsub!(/ (?:commands|recipe) for target /,
                 ' commands for target ')
  expected.gsub!(/ (?:commands|recipe) commences /,
                 ' commands commence ')
  expected.gsub!(' (did you mean TAB instead of 8 spaces?)', '')
  expected.gsub!('Extraneous text after', 'extraneous text after')
  # Not sure if this is useful.
  expected.gsub!(/\s+Stop\.$/, '')
  # GNU make 4.0 has this output.
  expected.gsub!(/Makefile:\d+: commands for target ".*?" failed\n/, '')
  # We treat some warnings as errors.
  expected.gsub!(/Nothing to be done for "test"\.\n/, '')

  expected
end

def normalize_kati_log(output)
  output = move_circular_dep(output)
  # kati specific log messages.
  output.gsub!(/^\*kati\*.*\n/, '')
  output.gsub!(/[`'"]/, '"')
  output.gsub!(/(: )open (\S+): n(o such file or directory)\nNOTE:.*/,
               "\\1\\2: N\\3\n*** No rule to make target \"\\2\".")
  output
end

run_make_test = proc do |mk|
  c = File.read(mk)
  expected_failure = c =~ /\A# TODO/

  run_in_testdir(mk) do |name|
    File.open("Makefile", 'w') do |ofile|
      ofile.print(c)
    end

    expected = ''
    output = ''

    testcases = c.scan(/^test\d*/).sort.uniq
    if testcases.empty?
      testcases = ['']
    end

    cleanup
    testcases.each do |tc|
      res = `make #{tc} 2>&1`
      res = normalize_make_log(res)
      expected += "=== #{tc} ===\n" + res
      expected_files = get_output_filenames
      expected += "\n=== FILES ===\n#{expected_files * "\n"}\n"
    end

    cleanup
    testcases.each do |tc|
      json = "#{tc.empty? ? 'test' : tc}"
      cmd = "../../kati -save_json=#{json}.json -kati_log #{tc} 2>&1"
      res = IO.popen(cmd, 'r:binary', &:read)
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
        puts `diff -u out.make out.kati`
        failures << name
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
        cmd = "../../kati -save_json=#{json}_2.json -load_json=#{json}.json -n -kati_log #{tc} 2>&1"
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
  run_in_testdir(sh) do |name|
    cleanup
    cmd = "sh ../../#{sh} make"
    expected = IO.popen(cmd, 'r:binary', &:read)
    cleanup
    cmd = "sh ../../#{sh} ../../kati --use_cache --kati_log"
    output = IO.popen(cmd, 'r:binary', &:read)

    expected = normalize_make_log(expected)
    output = normalize_kati_log(output)
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
  puts 'FAIL!'
else
  puts 'PASS!'
end
