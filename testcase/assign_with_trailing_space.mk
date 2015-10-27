a := Y # comment
$(info X$(a)Z)
a := Y 
$(info X$(a)Z)
a := Y	
$(info X$(a)Z)

sp := $(subst S, ,S)
a := Y$(sp)
$(info X$(a)Z)
a := $(sp)Y
$(info X$(a)Z)
