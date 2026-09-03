package config

import (
	"net"
	"testing"
)

func TestIsIntranetIP(t *testing.T) {
	yes := []string{"192.168.1.1", "10.0.0.1", "172.16.0.5", "127.0.0.1", "169.254.1.1", "::1", "fd00::1"}
	no := []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"}
	for _, s := range yes {
		if !IsIntranetIP(net.ParseIP(s)) {
			t.Fatalf("%s should be intranet", s)
		}
	}
	for _, s := range no {
		if IsIntranetIP(net.ParseIP(s)) {
			t.Fatalf("%s should not be intranet", s)
		}
	}
}
