package logreport

import (
	"bytes"
)

// ParseLTSV is simple ltsv parser
func ParseLTSV(data []byte, parseKey [][]byte) map[string][]byte {
	ret := make(map[string][]byte, len(parseKey))
	list := bytes.Split(data, []byte("\t"))
	for _, key := range parseKey {
		for _, b := range list {
			s := bytes.SplitN(b, []byte(":"), 2)
			if len(s) < 2 {
				continue
			}
			if bytes.Equal(s[0], key) {
				ret[string(key)] = s[1]
				break
			}
		}
	}
	return ret
}
