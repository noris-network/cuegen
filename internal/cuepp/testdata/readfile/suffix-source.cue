package readfile

data: {

	a_b_c:     string @readfile(a.txt=nl,b.txt=nl,c.txt)
	a_b_c_enc: string @readfile(a.txt.enc=nl,b.txt.enc=nl,c.txt.enc)

	newline_trim:  string @readfile(nl.txt=trim)
	newline_bytes: bytes  @readfile(nl.txt=bytes)
}
