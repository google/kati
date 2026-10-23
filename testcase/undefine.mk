# Undefining a variable is distinct from assigning it an empty value.
from_file = value
undefine from_file
$(info from_file=$(origin from_file):$(flavor from_file))
ifdef from_file
$(error from_file should be undefined)
endif

expanded_name = from_expansion
from_expansion = value
undefine $(expanded_name)
$(info from_expansion=$(origin from_expansion):$(flavor from_expansion))
ifdef from_expansion
$(error from_expansion should be undefined)
endif

overridden = value
override undefine overridden
$(info overridden=$(origin overridden):$(flavor overridden))
ifdef overridden
$(error overridden should be undefined)
endif

exported = value
export undefine exported
$(info exported=$(origin exported):$(flavor exported))
ifdef exported
$(error exported should be undefined)
endif

all:
