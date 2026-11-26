package textdoc

import (
	"testing"
	"time"
)

func TestFormatIsPreservedAndMixedEndingsAreReadOnly(test *testing.T) {
	modified := time.Unix(1700000000, 0)
	document, err := Decode("/a.txt", []byte("\ufeff一\r\n二\r\n"), modified, 0644)
	if err != nil || !document.BOM || document.LineEnding != "crlf" || document.Content != "一\r\n二\r\n" {
		test.Fatalf("decode: %+v %v", document, err)
	}
	encoded, err := Encode(document, "三\n四\n")
	if err != nil || string(encoded) != "\ufeff三\r\n四\r\n" {
		test.Fatalf("encode: %q %v", encoded, err)
	}
	other, _ := Decode("/a.txt", []byte("\ufeff一\r\n二\r\n"), modified.Add(time.Second), 0644)
	if other.Version == document.Version {
		test.Fatal("version ignores modification time")
	}
	mixed, _ := Decode("/b.txt", []byte("a\r\nb\nc\r"), modified, 0644)
	if mixed.LineEnding != "mixed" {
		test.Fatal("mixed endings not detected")
	}
	if _, err := Encode(mixed, "x"); err == nil {
		test.Fatal("mixed endings rewritten")
	}
	for _, data := range [][]byte{{0xff, 0xfe}, []byte("nul\x00")} {
		if _, err := Decode("/c", data, modified, 0644); err == nil {
			test.Fatalf("accepted %q", data)
		}
	}
	if _, err := Encode(document, string(make([]byte, Limit+1))); err == nil {
		test.Fatal("oversized content accepted")
	}
}
