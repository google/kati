x := one
x += two $(x)
$(info $(x))

# TODO: shouldn't crash.
#y = one
#y += two $(y)
#$(info $(y))