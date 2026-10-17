package mp4

import (
	"bytes"
	"encoding/binary"
	"os"
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
func TestInsertUDTA(t *testing.T) {
	// Build a minimal MP4: ftyp + moov(mvhd) + mdat
	var ftyp []byte
	// ftyp box: size=16
	ftyp = append(ftyp, 0,0,0,16)
	ftyp = append(ftyp, 'f','t','y','p')
	ftyp = append(ftyp, 'i','s','o','m') // brand
	ftyp = append(ftyp, 0,0,0,0)         // minor version
	// mvhd box: size=108 (enough for header)
	var mvhd []byte
	mvhd = append(mvhd, 0,0,0,108)
	mvhd = append(mvhd, 'm','v','h','d')
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0) // version+flags
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0) // creation/modification
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0) // rate/volume
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0) // matrix (9*4=36 bytes, zero)
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0) // pre_defined (6*4=24 bytes, zero)
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,1) // next_track_ID
	// moov box: contains mvhd
	var moov []byte
	moovSize := uint64(8 + len(mvhd)) // moov header (8) + mvhd
	var moovHdr [8]byte
	binary.BigEndian.PutUint32(moovHdr[:4], uint32(moovSize))
	moovHdr[4] = 'm'; moovHdr[5] = 'o'; moovHdr[6] = 'o'; moovHdr[7] = 'v'
	moov = append(moov, moovHdr[:]...)
	moov = append(moov, mvhd...)
	// mdat box: size=8
	var mdat []byte
	mdat = append(mdat, 0,0,0,8)
	mdat = append(mdat, 'm','d','a','t')
	// Assemble: ftyp + moov + mdat
	var data []byte
	data = append(data, ftyp...)
	data = append(data, moov...)
	data = append(data, mdat...)
	// udta box to insert (full box with header + payload)
	udtaPayload := []byte("test udta payload")
	var udtaBox []byte
	udtaSize := uint64(8 + len(udtaPayload))
	var udtaHdr [8]byte
	binary.BigEndian.PutUint32(udtaHdr[:4], uint32(udtaSize))
	udtaHdr[4] = 'u'; udtaHdr[5] = 'd'; udtaHdr[6] = 't'; udtaHdr[7] = 'a'
	udtaBox = append(udtaBox, udtaHdr[:]...)
	udtaBox = append(udtaBox, udtaPayload...)
	// Write to temp file
	tmpFile := os.TempDir() + "/vidproc_udta_test.mp4"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	defer os.Remove(tmpFile)
	// Insert udta box into moov
	if err := InsertUDTABox(tmpFile, udtaBox); err != nil {
		t.Fatalf("InsertUDTABox failed: %v", err)
	}
	// Read back
	out, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}
	// Verify file grew by udta box size
	expectedSize := 16 + int(moovSize) + 8 + len(udtaBox)
	if len(out) != expectedSize {
		t.Fatalf("expected size %d, got %d", expectedSize, len(out))
	}
	// Verify udta box exists in moov
	boxes := ScanBoxes(out)
	var moovBox *Box
	for _, b := range boxes {
		if b.Type == "moov" {
			moovBox = &b
			break
		}
	}
	if moovBox == nil {
		t.Fatalf("moov not found after insert")
	}
	inner := ScanBoxes(moovBox.Payload)
	var found bool
	for _, b := range inner {
		if b.Type == "udta" {
			found = true
			if !bytes.Equal(b.Payload, udtaPayload) {
				t.Fatalf("udta payload mismatch: got %s, want %s", b.Payload, udtaPayload)
			}
			break
		}
	}
	if !found {
		t.Fatalf("udta box not inserted into moov")
	}
}
func TestInsertUDTABox(t *testing.T) {
	// Build a minimal MP4: ftyp + moov(mvhd) + mdat
	var ftyp []byte
	// ftyp box: size=16
	ftyp = append(ftyp, 0,0,0,16)
	ftyp = append(ftyp, 'f','t','y','p')
	ftyp = append(ftyp, 'i','s','o','m') // brand
	ftyp = append(ftyp, 0,0,0,0)         // minor version
	// mvhd box: size=108 (enough for header)
	var mvhd []byte
	mvhd = append(mvhd, 0,0,0,108)
	mvhd = append(mvhd, 'm','v','h','d')
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0) // version+flags
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0) // creation/modification
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0) // rate/volume
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0) // matrix (9*4=36 bytes, zero)
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,0) // pre_defined (6*4=24 bytes, zero)
	mvhd = append(mvhd, 0,0,0,0, 0,0,0,1) // next_track_ID
	// moov box: contains mvhd
	var moov []byte
	moovSize := uint64(8 + len(mvhd)) // moov header (8) + mvhd
	var moovHdr [8]byte
	binary.BigEndian.PutUint32(moovHdr[:4], uint32(moovSize))
	moovHdr[4] = 'm'; moovHdr[5] = 'o'; moovHdr[6] = 'o'; moovHdr[7] = 'v'
	moov = append(moov, moovHdr[:]...)
	moov = append(moov, mvhd...)
	// mdat box: size=8
	var mdat []byte
	mdat = append(mdat, 0,0,0,8)
	mdat = append(mdat, 'm','d','a','t')
	// Assemble: ftyp + moov + mdat
	var data []byte
	data = append(data, ftyp...)
	data = append(data, moov...)
	data = append(data, mdat...)
	// udta box to insert (full box with header + payload)
	udtaPayload := []byte("test udta payload")
	var udtaBox []byte
	udtaSize := uint64(8 + len(udtaPayload))
	var udtaHdr [8]byte
	binary.BigEndian.PutUint32(udtaHdr[:4], uint32(udtaSize))
	udtaHdr[4] = 'u'; udtaHdr[5] = 'd'; udtaHdr[6] = 't'; udtaHdr[7] = 'a'
	udtaBox = append(udtaBox, udtaHdr[:]...)
	udtaBox = append(udtaBox, udtaPayload...)
	// Write to temp file
	tmpFile := os.TempDir() + "/vidproc_udta_box_test.mp4"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	defer os.Remove(tmpFile)
	// Insert udta box into moov
	if err := InsertUDTABox(tmpFile, udtaBox); err != nil {
		t.Fatalf("InsertUDTABox failed: %v", err)
	}
	// Read back
	out, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}
	// Verify file grew by udta box size
	expectedSize := 16 + int(moovSize) + 8 + len(udtaBox)
	if len(out) != expectedSize {
		t.Fatalf("expected size %d, got %d", expectedSize, len(out))
	}
	// Verify udta box exists in moov
	boxes := ScanBoxes(out)
	var moovBox *Box
	for _, b := range boxes {
		if b.Type == "moov" {
			moovBox = &b
			break
		}
	}
	if moovBox == nil {
		t.Fatalf("moov not found after insert")
	}
	inner := ScanBoxes(moovBox.Payload)
	var found bool
	for _, b := range inner {
		if b.Type == "udta" {
			found = true
			if !bytes.Equal(b.Payload, udtaPayload) {
				t.Fatalf("udta payload mismatch: got %s, want %s", b.Payload, udtaPayload)
			}
			break
		}
	}
	if !found {
		t.Fatalf("udta box not inserted into moov")
	}
}
