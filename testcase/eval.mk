test1:
	touch server.c server_priv.c server_access.c
	touch client.c client_api.c client_mem.c

test2: all

PROGRAMS    = server client

server_OBJS = server.o server_priv.o server_access.o

client_OBJS = client.o client_api.o client_mem.o

# Everything after this is generic

.PHONY: all
all: $(PROGRAMS)

define PROGRAM_template
 $(1): $$($(1)_OBJS)
 ALL_OBJS += $$($(1)_OBJS)
endef

$(foreach prog,$(PROGRAMS),$(eval $(call PROGRAM_template,$(prog))))

$(PROGRAMS):
	echo $^ -o $@

clean:
	rm -f $(ALL_OBJS) $(PROGRAMS)
