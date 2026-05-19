package handler

import (
	"strings"
)

func parseURI(uri string) (mType, name, value string) {
	parsed := make([]string, 3)
	s := strings.Split(uri, "/")
	s = s[2:]
	for i, v := range s {
		parsed[i] = v
	}
	
	return parsed[0], parsed[1], parsed[2]
}
