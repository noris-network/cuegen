// generated using https://github.com/zarldev/goenums

package cuegen

import "fmt"

type Kind struct {
	kind
}

type kind int

const (
	unknown kind = iota
	cuegen
	cuegenpackage
	cuegenlibrary
)

var (
	strKindArray = [...]string{
		cuegen:        "Cuegen",
		cuegenpackage: "CuegenPackage",
		cuegenlibrary: "CuegenLibrary",
	}

	typeKindMap = map[string]kind{
		"Cuegen":        cuegen,
		"CuegenPackage": cuegenpackage,
		"CuegenLibrary": cuegenlibrary,
	}
)

func (t kind) String() string {
	return strKindArray[t]
}

func Parse(a any) Kind {
	switch v := a.(type) {
	case Kind:
		return v
	case string:
		return Kind{stringToKind(v)}
	case fmt.Stringer:
		return Kind{stringToKind(v.String())}
	case int:
		return Kind{kind(v)}
	case int64:
		return Kind{kind(int(v))}
	case int32:
		return Kind{kind(int(v))}
	}
	return Kind{unknown}
}

func stringToKind(s string) kind {
	if v, ok := typeKindMap[s]; ok {
		return v
	}
	return unknown
}

func (t kind) IsValid() bool {
	return t >= kind(1) && t <= kind(len(strKindArray))
}

type kindsContainer struct {
	UNKNOWN       Kind
	Cuegen        Kind
	CuegenPackage Kind
	CuegenLibrary Kind
}

var Kinds = kindsContainer{
	UNKNOWN:       Kind{unknown},
	Cuegen:        Kind{cuegen},
	CuegenPackage: Kind{cuegenpackage},
	CuegenLibrary: Kind{cuegenlibrary},
}

func (c kindsContainer) All() []Kind {
	return []Kind{
		c.Cuegen,
		c.CuegenPackage,
		c.CuegenLibrary,
	}
}

func (t Kind) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

func (t *Kind) UnmarshalJSON(b []byte) error {
	*t = Parse(string(b))
	return nil
}
