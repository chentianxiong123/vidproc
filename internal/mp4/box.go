package mp4

import (
	"bytes"
	"encoding/binary"
	"os"
)

type Box struct {
	Type    string
	Offset  int64
	Size    uint64
	Payload []byte
}

// ScanBoxes parses the top-level MP4 box tree from data.
// Returns all boxes with their header and payload.
func ScanBoxes(data []byte) []Box {
	var boxes []Box
	off := 0
	for off+8 <= len(data) {
		size := binary.BigEndian.Uint32(data[off : off+4])
		typ := string(data[off+4 : off+8])
		hdr := 8
		var boxSize uint64

		switch size {
		case 0:
			if off+16 > len(data) {
				return boxes
			}
			size64 := binary.BigEndian.Uint64(data[off+8 : off+16])
			if size64 > 0xFFFFFFFF {
				return boxes
			}
			boxSize = size64
			hdr = 16
		case 1:
			if off+16 > len(data) {
				return boxes
			}
			size64 := binary.BigEndian.Uint64(data[off+8 : off+16])
			if size64 > 0xFFFFFFFF {
				return boxes
			}
			boxSize = size64
			hdr = 16
		default:
			boxSize = uint64(size)
		}

		if int64(off)+int64(boxSize) > int64(len(data)) {
			return boxes
		}

		payload := data[off+hdr : off+int(boxSize)]
		boxes = append(boxes, Box{
			Type:    typ,
			Offset:  int64(off),
			Size:    boxSize,
			Payload: payload,
		})

		off += int(boxSize)
	}
	return boxes
}

// FilterBoxes filters boxes by type and/or substring match on payload.
// If dedup is true, duplicate payloads (by content) are removed, keeping the first occurrence.
func FilterBoxes(boxes []Box, boxType, matchSubstr string, dedup bool) []Box {
	var filtered []Box
	seen := make(map[string]bool)

	for _, b := range boxes {
		if boxType != "" && b.Type != boxType {
			continue
		}
		if matchSubstr != "" && !bytes.Contains(b.Payload, []byte(matchSubstr)) {
			continue
		}
		if dedup {
			key := string(b.Payload)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		filtered = append(filtered, b)
	}
	return filtered
}

// Standard boxes that are part of the MP4 spec.
var StandardBoxes = map[string]bool{
	"ftyp": true, "moov": true, "mdat": true, "moof": true,
	"styp": true, "sidx": true, "prft": true,
}

// NonStandardBoxes returns only boxes that are NOT standard MP4 boxes.
func NonStandardBoxes(boxes []Box) []Box {
	var out []Box
	for _, b := range boxes {
		if !StandardBoxes[b.Type] {
			out = append(out, b)
		}
	}
	return out
}

// ExtractPayloads concatenates the payloads of the given boxes.
func ExtractPayloads(boxes []Box) []byte {
	var out []byte
	for _, b := range boxes {
		out = append(out, b.Payload...)
	}
	return out
}

// AppendBoxes appends the full box data (header + payload) to the destination file.
// This preserves the box structure for later parsing.
func AppendBoxes(dstPath string, boxes []Box) error {
	// Build the full box data (header + payload) for each box
	var data []byte
	for _, b := range boxes {
		// Box header: size (4 bytes) + type (4 bytes)
		size := uint32(8 + len(b.Payload))
		data = append(data,
			byte(size>>24), byte(size>>16), byte(size>>8), byte(size),
		)
		data = append(data, b.Type...)
		data = append(data, b.Payload...)
	}

	// Append to file
	f, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
