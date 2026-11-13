package persona

import (
	"fmt"
	"strings"
	"testing"
)

func TestForNodeIsDeterministic(t *testing.T) {
	first := ForNode("ztabcdef")
	second := ForNode("ztabcdef")
	if first.UA != second.UA || first.SecChUa != second.SecChUa || first.ClientHello.Str() != second.ClientHello.Str() {
		t.Fatalf("persona changed between calls:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestForNodeSpreadsAcrossPool(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		p := ForNode(fmt.Sprintf("node-%02d", i))
		seen[p.UA] = true
	}
	if len(seen) < 4 {
		t.Fatalf("persona spread = %d unique UAs, want at least 4", len(seen))
	}
}

func TestPoolKeepsHeadersAligned(t *testing.T) {
	for _, p := range Pool() {
		if strings.Contains(p.UA, "Chrome/120.") && (!strings.Contains(p.SecChUa, `v="120"`) || p.ClientHello.Str() != "Chrome-120") {
			t.Fatalf("Chrome 120 persona mismatch: %+v", p)
		}
		if strings.Contains(p.UA, "Chrome/131.") && (!strings.Contains(p.SecChUa, `v="131"`) || p.ClientHello.Str() != "Chrome-131") {
			t.Fatalf("Chrome 131 persona mismatch: %+v", p)
		}
	}
}
