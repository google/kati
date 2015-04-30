package main

var symtab = make(map[string]string)

func intern(s string) string {
	if v, ok := symtab[s]; ok {
		return v
	}
	symtab[s] = s
	return s
}

func internBytes(s []byte) string {
	if v, ok := symtab[string(s)]; ok {
		return v
	}
	v := string(s)
	symtab[v] = v
	return v
}
