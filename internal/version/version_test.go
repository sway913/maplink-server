package version

import "testing"

func TestValueIsSet(t *testing.T) {
	if Value == "" {
		t.Fatal("version must not be empty")
	}
}
