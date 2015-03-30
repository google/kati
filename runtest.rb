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

    c.scan(/^test\d*/).sort.each do |tc|
      cleanup
      expected += "=== #{tc} ===\n" + `make 2>&1`
      expected_files = get_output_filenames
      cleanup
      output += "=== #{tc} ===\n" + `../../kati 2>&1`
      output_files = get_output_filenames

      expected.gsub!(/^make\[.*\n/, '')
      output.gsub!(/^\*kati\*.*\n/, '')

      expected += "\n=== FILES ===\n#{expected_files * "\n"}\n"
      output += "\n=== FILES ===\n#{output_files * "\n"}\n"
    end

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
