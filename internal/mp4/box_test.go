package mp4

import (
	"bytes"
	"testing"
)

func TestScanBoxes(t *testing.T) {
	// Construct a simple MP4 with two top-level boxes: ftyp (standard) and free (custom) containing a known string.
	var data []byte
	// ftyp box: size=16, type="ftyp", then 4 bytes brand + 4 bytes minor version + 4 bytes compatible_brands
	data = append(data, 0, 0, 0, 16)
	data = append(data, 'f', 't', 'y', 'p')
	data = append(data, 'i', 's', 'o', 'm') // brand
	data = append(data, 0, 0, 0, 0)         // minor version

	// free box: size=8+len(payload), type="free", header 8 bytes, payload "hello world"
	payload := []byte("hello world")
	size := uint64(8 + len(payload)) // header 8 + payload
	data = append(data, byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	data = append(data, 'f', 'r', 'e', 'e')
	// payload
	data = append(data, payload...)

	boxes := ScanBoxes(data)
	if len(boxes) != 2 {
		t.Fatalf("expected 2 boxes, got %d", len(boxes))
	}
	if boxes[0].Type != "ftyp" || boxes[0].Offset != 0 || boxes[0].Size != 16 {
		t.Fatalf("first box wrong: %+v", boxes[0])
	}
	if boxes[1].Type != "free" || boxes[1].Offset != 16 || boxes[1].Size != uint64(8+len(payload)) {
		t.Fatalf("second box wrong: %+v", boxes[1])
	}
	if !bytes.Equal(boxes[1].Payload, payload) {
		t.Fatalf("payload mismatch: got %s, want %s", boxes[1].Payload, payload)
	}
}

func TestFilterBoxes(t *testing.T) {
	boxes := []Box{
		{Type: "ftyp", Payload: []byte{}},
		{Type: "free", Payload: []byte("hello")},
		{Type: "free", Payload: []byte("world")},
		{Type: "moov", Payload: []byte{}},
	}
	// filter by type free
	filt := FilterBoxes(boxes, "free", "", false)
	if len(filt) != 2 {
		t.Fatalf("expected 2 free boxes, got %d", len(filt))
	}
	// filter by substring "hell"
	filt2 := FilterBoxes(boxes, "", "hell", false)
	if len(filt2) != 1 || !bytes.Equal(filt2[0].Payload, []byte("hello")) {
		t.Fatalf("expected box with 'hello', got %v", filt2)
	}
	// filter by type free and substring "orl"
	filt3 := FilterBoxes(boxes, "free", "orl", false)
	if len(filt3) != 1 || !bytes.Equal(filt3[0].Payload, []byte("world")) {
		t.Fatalf("expected box with 'world', got %v", filt3)
	}
	// deduplication test: two identical payloads
	boxes2 := []Box{
		{Type: "free", Payload: []byte("same")},
		{Type: "free", Payload: []byte("same")},
		{Type: "free", Payload: []byte("different")},
	}
	filt4 := FilterBoxes(boxes2, "free", "", true) // dedup true
	if len(filt4) != 2 {
		t.Fatalf("expected 2 boxes after dedup, got %d", len(filt4))
	}
	// check that we have one "same" and one "different"
	sameCount := 0
	diffCount := 0
	for _, b := range filt4 {
		if bytes.Equal(b.Payload, []byte("same")) {
			sameCount++
		} else if bytes.Equal(b.Payload, []byte("different")) {
			diffCount++
		}
	}
	if sameCount != 1 || diffCount != 1 {
		t.Fatalf("expected one same and one different, got same=%d diff=%d", sameCount, diffCount)
	}
}

func TestNonStandardBoxes(t *testing.T) {
	boxes := []Box{
		{Type: "ftyp", Payload: []byte{}},
		{Type: "free", Payload: []byte("custom")},
		{Type: "moov", Payload: []byte{}},
	}
	ns := NonStandardBoxes(boxes)
	if len(ns) != 1 {
		t.Fatalf("expected 1 non-standard box, got %d", len(ns))
	}
	if ns[0].Type != "free" {
		t.Fatalf("expected free box, got %s", ns[0].Type)
	}
}
