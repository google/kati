R"N_FrH57_hmVvbWOM(
ckati - An experimental GNU make clone.
Usage: ckati [OPTIONS] [OTHER OPTS]

Positionals:
  OTHER OPTS TEXT             restOpts

Options:
  -h,--help                   Print this help message and exit
[Option Group: make]
  options mimicking ones of Make or just relevant to using kati as make
  Options:
    -f,--file TEXT              file
    -s,--silent                 silent mode. currently does nothing. It controls global_echo variable but it doesn't control anything.
    -j,--jobs INT:POSITIVE      num of processes
    -i                          dry run, don't run any commands from makefiles
    --no_builtin_rules          Do not insert `.c.o`, `.cc.o` and some GNU Make advertising
    -c                          only check makefile syntax, don't do anything else
[Option Group: ninja]
  options for using with ninja
  Options:
    --no_ninja_prelude          Disables outputing some lines into `build.ninja`. These lines are about ninja binary, "pool local_pool", count of jobs and _kati_always_build_ rule.
    --ninja                     Transform a makefile into `build.ninja`.
    --empty_ninja_file          outputs something additional into `build.ninja`
    --ninja_suffix TEXT         Something added into end of files names. Both input and output filenames are affected.
    --ninja_dir TEXT            directory of ninja binary
    --remote_num_jobs INT:POSITIVE
                                count of jobs for ninja
    --detect_android_echo       disable .phony for Android
    --default_pool TEXT         uses this pool name instead of `local_pool`
    --use_ninja_phony_output BOOLEAN
                                use sets phony_output = true instead of using _kati_always_build_
    --use_ninja_validations     
[Option Group: kati]
  settings of kati itself
  Options:
    --version                   print version of this program
    --regen_ignoring_kati_binary
    --color_warnings            show warnings using ECMA control codes
    --gen_all_targets           
    --regen                     Check if regeneration is needed
    --top_level_phony           
    --ignore_optional_include TEXT
                                don't include a file if it matches this pattern
    --ignore_dirty TEXT         files matching this pattern are not regenerated
    --no_ignore_dirty TEXT      files matching this pattern are regenerated even if ignored by ignore_dirty
    --writable TEXT ...         List of some prefixes of file names.
[Option Group: features]
  additional features
  Options:
    --goma_dir TEXT             Directory of gomacc binary. See https://chromium.googlesource.com/infra/goma/client/+/master/README.md for more info.
    --use_find_emulator         emulate `find` command
    --detect_depfiles           detects dependent files by looking for filenames looking like artifacts in commands
[Option Group: debug]
  flags useful for debugging
  Options:
    -d,--debug                  print debug messages
    --kati_stats                Enable stat logs
    --regen_debug               print info about dirtiness status of files and about the decisions amde
    --dump_kati_stamp           
[Option Group: warnings]
  enable warnings
  Options:
    --warn                      Enable warnings
    --werror_find_emulator      
    --werror_overriding_commands
    --warn_implicit_rules       
    --werror_implicit_rules     
    --warn_suffix_rules         
    --werror_suffix_rules       
    --warn_real_to_phony        
    --werror_real_to_phony      
    --warn_phony_looks_real     
    --werror_phony_looks_real   
    --werror_writable           
    --warn_real_no_cmds_or_deps BOOLEAN
    --werror_real_no_cmds_or_deps
    --warn_real_no_cmds BOOLEAN 
    --werror_real_no_cmds       

)N_FrH57_hmVvbWOM"
