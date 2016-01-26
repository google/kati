srcs := a.cc b.cc c.cc
srcs := $(addprefix ./,$(srcs))
objs := $(patsubst ./%.cc,./%.o,$(srcs))

test: out

out: $(objs)

$(objs): ./%.o: ./%.cc
	echo $@: $<: $^

a.o: a.cc a.h
b.o: b.cc a.h b.h
c.o: b.cc a.h b.h c.h

