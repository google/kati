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

all: ckati ckati_tests

include Makefile.kati
include Makefile.ckati

info: ckati
	@echo GNU MAKE VERSION
	make --version
	make -f Makefile version --no-print-directory
	@echo
	@echo CKATI VERSION
	./ckati -f Makefile version
	@echo
	@echo BASH VERSION
	-/bin/bash --version | head -n 1
	@echo
	@echo SHELL VERSION
	@echo $(SHELL)
	$(SHELL) --version | head -n 1

version:
	@echo $(MAKE_VERSION)

test: all ckati_tests
	go test --ckati
	go test --ckati --ninja

clean: ckati_clean

.PHONY: test clean ckati_tests
