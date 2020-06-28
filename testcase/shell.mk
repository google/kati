test:
	echo $(shell dirname $$(pwd))
	echo $(shell false)
	echo $(shell  /bin/echo -e "\na \n  b 	\n " )
	echo $(shell  /bin/echo -e "\na \n  b 	\n " )X
	echo X$(shell  /bin/echo -e "\n\n" )Y
	echo X$(shell  /bin/echo -e "a\n\n" )Y
	echo X$(shell  /bin/echo -e "\n\nb" )Y
	echo X$(shell  /bin/echo -e "\n\nb" )Y
	echo X$(shell  /bin/echo -e "\n\n\nb" )Y
	echo X$(shell  /bin/echo -e "   b" )Y
	echo X$(shell  /bin/echo -e "b   " )Y
