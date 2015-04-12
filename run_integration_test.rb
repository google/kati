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
    # TODO: Fix.
    if @name == 'android'
      log = log.gsub(/[ \t]+/, ' ')
    end
    log = log.split("\n").sort.join("\n").sub(/ Stop\.$/, '')
    # This is a completely sane warning from kati for Android.
    log.sub!(%r(build/core/product_config.mk:152: warning: Unmatched parens: .*\n), '')
    # Not sure why the order can be inconsistent, but this would be OK.
    # TODO: Inevestigate.
    log.gsub!(/(\.mk\.PRODUCT_COPY_FILES := )(.*)/){$1 + $2.split.sort * ' '}
    File.open(out, 'w') do |of|
      of.print log
    end
    log
  end

  def run
    @checkout.call(self)

    Dir.chdir(@name) do
      @prepare.call(self)

      [['make', 'make'], ['kati', '../../kati']].each do |n, cmd|
        @clean.call(self)
        print "Running #{n} for #{@name}..."
        STDOUT.flush
        started = Time.now
        system("#{cmd} #{@target} > #{n}.log 2>&1")
        elapsed = Time.now - started
        puts " %.2f secs" % elapsed
      end

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
      md5 = `md5sum android.tgz`
      need_update = true
      if File.exist?("#{@name}/STAMP")
        stamp = File.read("#{@name}/STAMP")
        if md5 == stamp
          need_update = false
        end
      end

      if need_update
        check_command("tar -xzf android.tgz")
        File.open("#{@name}/STAMP.tmp", 'w') do |ofile|
          ofile.print(md5)
        end
        File.rename("#{@name}/STAMP.tmp", "#{@name}/STAMP")
      end
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
