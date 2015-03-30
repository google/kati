#!/usr/bin/env ruby

require 'fileutils'

def get_output_filenames
  files = Dir.glob('*')
  files.delete('Makefile')
  files
end

def cleanup
  get_output_filenames.each do |fname|
    FileUtils.rm fname
  end
end

Dir.glob('test/*.mk').sort.each do |mk|
  c = File.read(mk)

  expected_failure = c =~ /\A# TODO/

  name = mk[/([^\/]+)\.mk$/, 1]
  dir = "out/#{name}"
  FileUtils.rm_rf(dir)
  FileUtils.mkdir_p(dir)

  Dir.chdir(dir) do
    File.open("Makefile", 'w') do |ofile|
      ofile.print(c)
    end

    expected = ''
    output = ''

    testcases = c.scan(/^test\d*/).sort
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
      output += "=== #{tc} ===\n" + `../../kati #{tc} 2>&1`
      output_files = get_output_filenames
      output += "\n=== FILES ===\n#{output_files * "\n"}\n"
    end

    expected.gsub!(/^make\[.*\n/, '')
    # Normalizations for old/new GNU make.
    expected.gsub!(/[`'"]/, '"')
    expected.gsub!(/ (?:commands|recipe) for target /,
                   ' commands for target ')
    expected.gsub!(' (did you mean TAB instead of 8 spaces?)', '')
    expected.gsub!(/\s+Stop\.$/, '')
    output.gsub!(/^\*kati\*.*\n/, '')

    File.open('out.make', 'w'){|ofile|ofile.print(expected)}
    File.open('out.kati', 'w'){|ofile|ofile.print(output)}

    if expected != output
      if expected_failure
        puts "#{name}: FAIL (expected)"
      else
        puts "#{name}: FAIL"
        puts `diff -u out.make out.kati`
      end
    else
      if expected_failure
        puts "#{name}: PASS (unexpected)"
      else
        puts "#{name}: PASS"
      end
    end
  end
end
