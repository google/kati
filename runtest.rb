#!/usr/bin/env ruby

require 'fileutils'

def get_output_filenames
  files = Dir.glob('*')
  files.delete('Makefile')
  files
end

def cleanup
  get_output_filenames.each do |fname|
    FileUtils.rm_rf fname
  end
end

expected_failures = []
unexpected_passes = []
failures = []
passes = []

Dir.glob('testcase/*.mk').sort.each do |mk|
  c = File.read(mk)

  expected_failure = c =~ /\A# TODO/

  name = mk[/([^\/]+)\.mk$/, 1]
  dir = "out/#{name}"

  FileUtils.mkdir_p(dir)
  Dir.glob("#{dir}/*").each do |fname|
    FileUtils.rm_rf(fname)
  end

  Dir.chdir(dir) do
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
      expected += "=== #{tc} ===\n" + `make #{tc} 2>&1`
      expected_files = get_output_filenames
      expected += "\n=== FILES ===\n#{expected_files * "\n"}\n"
    end

    cleanup
    testcases.each do |tc|
      output += "=== #{tc} ===\n" + `../../kati -kati_log #{tc} 2>&1`
      output_files = get_output_filenames
      output += "\n=== FILES ===\n#{output_files * "\n"}\n"
    end

    expected.gsub!(/^make\[\d+\]: (Entering|Leaving) directory.*\n/, '')
    expected.gsub!(/^make\[\d+\]: /, '')
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

    # kati specific log messages.
    output.gsub!(/^\*kati\*.*\n/, '')
    output.gsub!(/[`'"]/, '"')

    File.open('out.make', 'w'){|ofile|ofile.print(expected)}
    File.open('out.kati', 'w'){|ofile|ofile.print(output)}

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
