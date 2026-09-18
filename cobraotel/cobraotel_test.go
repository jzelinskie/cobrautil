package cobraotel

import "testing"

func TestNewExplicitServiceName(t *testing.T) {
	b := New("example-service")
	if b.serviceName != "example-service" {
		t.Fatalf("serviceName = %q, want example-service", b.serviceName)
	}
}
