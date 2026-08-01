package lyrics

import (
	"strings"
	"testing"
)

const wordTTML = `<tt xmlns:itunes="http://www.apple.com/itunes" itunes:timing="Word">
<body><div><p begin="00:01.000" end="00:03.000" itunes:key="L1">
<span begin="00:01.000" end="00:02.000">Hello</span> <span begin="00:02.000" end="00:03.000">world</span>
</p></div></body></tt>`

func TestTtmlToLrcWordMode(t *testing.T) {
	result, err := TtmlToLrc(wordTTML, "word")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "<00:01.00>Hello<00:02.00>") {
		t.Fatalf("expected word timestamps, got %q", result)
	}
}

func TestTtmlToLrcLineMode(t *testing.T) {
	result, err := TtmlToLrc(wordTTML, "line")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "<00:") {
		t.Fatalf("line mode contains word timestamps: %q", result)
	}
	if !strings.Contains(result, "[00:01.00]Hello world[00:03.00]") {
		t.Fatalf("expected line timestamps, got %q", result)
	}
}
