package main

import "testing"

func TestValidTwoFactorCode(t *testing.T) {
	tests := map[string]bool{
		"123456":  true,
		"000000":  true,
		"12345":   false,
		"1234567": false,
		"12a456":  false,
		"":        false,
	}
	for code, expected := range tests {
		if actual := validTwoFactorCode(code); actual != expected {
			t.Errorf("validTwoFactorCode(%q) = %v, want %v", code, actual, expected)
		}
	}
}

func TestLogBufferContainsSince(t *testing.T) {
	var logs logBuffer
	_, _ = logs.Write([]byte("old line\n"))
	mark := logs.mark()
	_, _ = logs.Write([]byte("Waiting for input...\n"))
	if logs.containsSince(mark, "old line") {
		t.Fatal("matched a line before the mark")
	}
	if !logs.containsSince(mark, "Waiting for input") {
		t.Fatal("did not match a line after the mark")
	}
}
