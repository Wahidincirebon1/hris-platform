package httpapi

import (
	"testing"
)

func TestCanUseEmployeeSelfService(t *testing.T) {
	for _, role := range []string{"employee", "manager", "hr"} {
		if !canUseEmployeeSelfService(role) {
			t.Errorf("role %q should have self-service access", role)
		}
	}
	for _, role := range []string{"finance", "superadmin", "unknown"} {
		if canUseEmployeeSelfService(role) {
			t.Errorf("role %q should not have self-service access", role)
		}
	}
}

func TestValidRequestTime(t *testing.T) {
	for _, value := range []string{"00:00", "08:30", "23:59"} {
		if !validRequestTime(value) {
			t.Errorf("validRequestTime(%q) = false", value)
		}
	}
	for _, value := range []string{"", "8:30", "24:00", "12:60", "noon"} {
		if validRequestTime(value) {
			t.Errorf("validRequestTime(%q) = true", value)
		}
	}
}
