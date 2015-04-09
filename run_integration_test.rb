#!/usr/bin/env ruby

require 'fileutils'

FileUtils.mkdir_p('repo')
Dir.chdir('repo')

def check_command(cmd)
  puts cmd
  if !system(cmd)
    puts "#{cmd} failed"
    exit 1
  end
end

class TestCase
  attr_reader :name

  def initialize(name, checkout, prepare, clean, target)
    @name = name
    @checkout = checkout
    @prepare = prepare
    @clean = clean
    @target = target
  end

  def normalize_log(log, out)
    log = log.gsub(/[ \t]+/, ' ').split("\n").sort.join("\n").sub(/ Stop\.$/, '')
    # This is a completely sane warning from kati for Android.
    log.sub!(%r(build/core/product_config.mk:152: warning: Unmatched parens: .*\n), '')
    File.open(out, 'w') do |of|
      of.print log
    end
    log
  end

  def run
    @checkout.call(self)

    Dir.chdir(@name) do
      @prepare.call(self)

      @clean.call(self)
      puts "Running make for #{@name}..."
      system("make #{@target} > make.log 2>&1")

      @clean.call(self)
      puts "Running kati for #{@name}..."
      system("../../kati #{@target} > kati.log 2>&1")

      make_log = File.read('make.log')
      kati_log = File.read('kati.log')
      kati_log.gsub!(/^\*kati\*.*\n/, '')

      make_log = normalize_log(make_log, 'make-normalized.log')
      kati_log = normalize_log(kati_log, 'kati-normalized.log')
      if make_log == kati_log
        puts "#{@name}: OK"
        return true
      else
        puts "#{@name}: FAIL"
        return false
      end
    end
  end
end

class GitTestCase < TestCase
  def initialize(name, repo, rev, prepare, clean, target)
    checkout = Proc.new{|tc|
      if !File.exist?(@name)
        check_command("git clone #{repo}")
      end
      Dir.chdir(@name) {
        check_command("git checkout #{rev}")
      }
    }

    super(name, checkout, prepare, clean, target)
  end
end

class AndroidTestCase < TestCase
  def initialize
    name = 'android'
    checkout = Proc.new{|tc|
      FileUtils.mkdir_p(@name)
      Dir.chdir(@name) {
        check_command("tar -xzf ../android.tgz")
      }
    }

    super(name, checkout, DO_NOTHING, DO_NOTHING, 'dump-products')
  end
end

DO_NOTHING = Proc.new{|tc|}
MAKE_CLEAN = Proc.new{|tc|
  check_command("make clean > /dev/null")
}
CONFIGURE = Proc.new{|tc|
  check_command("./configure > /dev/null")
}

TESTS = [
    GitTestCase.new('maloader',
                    'https://github.com/shinh/maloader.git',
                    '5d125933bc6c141bed05c309c2dc0e14ada6f5c7',
                    DO_NOTHING,
                    MAKE_CLEAN,
                    ''),
    GitTestCase.new('glog',
                    'https://github.com/google/glog',
                    '1b0b08c8dda1659027677966b03a3ff3c488e549',
                    CONFIGURE,
                    MAKE_CLEAN,
                    ''),
   AndroidTestCase.new(),
]

fails = []
TESTS.each do |tc|
  if !ARGV.empty?
    if !ARGV.include?(tc.name)
      next
    end
  end

  if !tc.run
    fails << tc.name
  end
end

puts

if fails.empty?
  puts "PASS!"
else
  puts "=== Failures ==="
  fails.each do |n|
    puts n
  end
  puts

  puts "FAIL!"
end
