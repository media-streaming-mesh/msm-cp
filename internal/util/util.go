package util

import (
	"fmt"
	"net"
	"strings"
)

func GetRemoteIPv4Address(url string) string {
	res := strings.ReplaceAll(url, "[", "")
	res = strings.ReplaceAll(res, "]", "")
	n := strings.LastIndex(res, ":")

	return fmt.Sprintf("%s", net.ParseIP(res[:n]))
}

func GetConnectionKey(s1, s2 string) string {
	return fmt.Sprintf("%s%s", s1, s2)
}
