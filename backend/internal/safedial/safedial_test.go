package safedial

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.1.2.3", "192.168.0.1", "172.16.5.5", "169.254.169.254", "::1", "fc00::1", "0.0.0.0"}
	for _, s := range blocked {
		if !IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s debería bloquearse", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "203.0.113.10", "2606:4700:4700::1111"}
	for _, s := range allowed {
		if IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s NO debería bloquearse", s)
		}
	}
}

func TestControlBlocksAndAllows(t *testing.T) {
	if err := Control("tcp", "169.254.169.254:80", nil); err == nil {
		t.Error("debería bloquear la IP de metadatos (link-local)")
	}
	if err := Control("tcp", "127.0.0.1:443", nil); err == nil {
		t.Error("debería bloquear loopback")
	}
	if err := Control("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("una IP pública no debería bloquearse: %v", err)
	}
}
