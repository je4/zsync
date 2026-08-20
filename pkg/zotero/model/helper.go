package model

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

var regexpTextVariables = regexp.MustCompile(`([a-zA-Z0-9_]+:([^ ` + "\n" + `<"]+|"[^"]+"))`)
var regexpRemoveEmpty = regexp.MustCompile(`(?m)^\s*$[\r\n]*|[\r\n]+\s+\z`)

func Text2Metadata(str string) map[string][]string {
	meta := map[string][]string{}
	if slices := regexpTextVariables.FindAllString(str, -1); slices != nil {
		for _, slice := range slices {
			kv := strings.Split(slice, ":")
			if len(kv) != 2 {
				continue
			}
			if _, ok := meta[kv[0]]; !ok {
				meta[kv[0]] = []string{}
			}
			meta[kv[0]] = append(meta[kv[0]], strings.TrimSpace(strings.Trim(kv[1], ` "`)))
		}
	}
	return meta
}

func TextNoMeta(str string) string {
	h := regexpTextVariables.ReplaceAllString(str, " ")
	h = regexpRemoveEmpty.ReplaceAllString(h, "")
	return h
}

func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func FolderExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func FmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}
