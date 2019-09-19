package zotero

import (
	"math/rand"
	"strings"
)

func CreateKey() string {
	return randomString(8, "key", true)
}

// https://github.com/zotero/dataserver/blob/master/model/DataObjectUtilities.inc.php#L63
func randomString(length int64, mode string, exclude_ambiguous bool) string {
	// if you want extended ascii, then add the characters to the array
	upper := []rune{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'}
	lower := []rune{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'}
	numbers := []rune{'2', '3', '4', '5', '6', '7', '8', '9'}
	ambigious := []rune{'l', '1', '0', 'O'}

	var chars []rune
	switch mode {
	case "key":
		chars = append(upper, numbers...)
	case "mixed":
		chars = append(lower, upper...)
		chars = append(chars, numbers...)
	case "upper":
		chars = upper
	case "lower":
		chars = lower
	}
	if !exclude_ambiguous && mode != "key" {
		chars = append(chars, ambigious...)
	}
	b := strings.Builder{}
	b.Grow(int(length))
	for i := int64(0); i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}
