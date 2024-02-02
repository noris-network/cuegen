package readfile

data: {
	abc:     string @readfile(a.txt,b.txt,c.txt)
	abc_enc: string @readfile(a.txt.enc,b.txt.enc,c.txt.enc)
}
