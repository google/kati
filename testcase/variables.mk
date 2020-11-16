
SET_BEFORE := should_not_appear_in_output_before

# Save .VARIABLES so we can filter out all the built-in stuff later
BEFORE:=$(.VARIABLES)

# Simple variable
ONE := 1

# This is := so $(1) is evaluable right now
EVALUABLE := $(ONE)$(1)

# Deferred execution with $(1), so call it a function
LOOKS_LIKE_A_FUNCTION_1 = $(1)
LOOKS_LIKE_A_FUNCTION_2 = $(ONE)$(1)

# Deferred execution without $(1), so should not be a function
NOT_A_FUNCTION_1 = SIMPLE_TEXT
NOT_A_FUNCTION_2 = $(ONE)

# We can't evaluate it without eval, so we assume that it *is* a function.
THE_EDGE_CASE_1 = $($(ONE))
THE_EDGE_CASE_2 = $($(SET_BEFORE))
THE_EDGE_CASE_3 = asdf$($(SET_BEFORE))
THE_EDGE_CASE_4 = $($(SET_BEFORE))fsda
THE_EDGE_CASE_5 = fdsa$($(SET_BEFORE))fdsa

# This was already set before we saved the snapshot, so it shouldn't
# reappear
SET_BEFORE += should_not_appear_in_output_before

$(info .VARIABLES (from make): $(sort $(filter-out $(BEFORE), $(.VARIABLES))))
$(info .VARIABLES (hard coded): BEFORE EVALUABLE LOOKS_LIKE_A_FUNCTION_1 LOOKS_LIKE_A_FUNCTION_2 NOT_A_FUNCTION_1 NOT_A_FUNCTION_2 ONE THE_EDGE_CASE_1 THE_EDGE_CASE_2 THE_EDGE_CASE_3 THE_EDGE_CASE_4 THE_EDGE_CASE_5)

ifdef KATI
$(info .KATI_SYMBOLS: $(sort $(filter-out $(BEFORE), $(.KATI_SYMBOLS))))
else
# Make doesn't support .VARIABLES so output the expected values manually
# for comparison
$(info .KATI_SYMBOLS: BEFORE EVALUABLE NOT_A_FUNCTION_1 NOT_A_FUNCTION_2 ONE)
endif

# Updating this variable should not cause it to appear
SET_BEFORE += a_new_value
