package zentao

func commaJoin(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += `"` + s + `"`
	}
	return out
}
