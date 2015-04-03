test:
	echo $(shell pwd)
	echo $(shell false)
	echo $(shell  echo -e "\na \n  b 	\n " )
	echo $(shell  echo -e "\na \n  b 	\n " )X
	echo X$(shell  echo -e "\n\n" )Y
	echo X$(shell  echo -e "a\n\n" )Y
	echo X$(shell  echo -e "\n\nb" )Y
	echo X$(shell  echo -e "\n\nb" )Y
	echo X$(shell  echo -e "\n\n\nb" )Y
	echo X$(shell  echo -e "   b" )Y
	echo X$(shell  echo -e "b   " )Y
