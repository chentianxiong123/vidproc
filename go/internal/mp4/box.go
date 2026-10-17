package mp4

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
// InsertUDTA inserts a udta box with the given payload as a child of the moov box.
// It reads the entire file, locates the moov box, inserts the udta box at the
// beginning of the moov's payload, and updates the moov size accordingly.
func InsertUDTA(dstPath string, udtaPayload []byte) error {
	data, err := os.ReadFile(dstPath)
	if err != nil {
		return err
	}
	boxes := ScanBoxes(data)
	var moov *Box
	for i := range boxes {
		if boxes[i].Type == "moov" {
			moov = &boxes[i]
			break
		}
	}
	if moov == nil {
		return fmt.Errorf("moov box not found")
	}
	// udta box size: 8 byte header + payload
	udtaSize := uint64(8 + len(udtaPayload))
	// construct udta box bytes
	var udtaBuf bytes.Buffer
	binary.Write(&udtaBuf, binary.BigEndian, udtaSize)
	udtaBuf.WriteString("udta")
	if _, err := udtaBuf.Write(udtaPayload); err != nil {
		return err
	}
	insertOff := moov.Offset + 8 // after moov header
	// splice: [0:insertOff] + udtaBuf + [insertOff:]
	newData := make([]byte, 0, len(data)+int(udtaSize))
	newData = append(newData, data[:insertOff]...)
	newData = append(newData, udtaBuf.Bytes()...)
	newData = append(newData, data[insertOff:]...)
	// update moov size in newData
	// moov size field is at moov.Offset, 4 bytes (assuming normal size < 4GB)
	sizeOff := moov.Offset
	binary.BigEndian.PutUint32(newData[sizeOff:sizeOff+4], uint32(moov.Size+udtaSize))
	return os.WriteFile(dstPath, newData, 0o644)
}
// InsertUDTABox inserts a udta box (with header and payload) as a child of the moov box.
// It reads the entire file, locates the moov box, inserts the udta box at the
// beginning of the moov's payload, and updates the moov size accordingly.
func InsertUDTABox(dstPath string, udtaBox []byte) error {
	data, err := os.ReadFile(dstPath)
	if err != nil {
		return err
	}
	boxes := ScanBoxes(data)
	var moov *Box
	for i := range boxes {
		if boxes[i].Type == "moov" {
			moov = &boxes[i]
			break
		}
	}
	if moov == nil {
		return fmt.Errorf("moov box not found")
	}
	// udtaBox should be a full box (header+payload)
	if len(udtaBox) < 8 {
		return fmt.Errorf("udta box too small: %d bytes", len(udtaBox))
	}
	insertOff := moov.Offset + 8 // after moov header
	// splice: [0:insertOff] + udtaBox + [insertOff:]
	newData := make([]byte, 0, len(data)+len(udtaBox))
	newData = append(newData, data[:insertOff]...)
	newData = append(newData, udtaBox...)
	newData = append(newData, data[insertOff:]...)
	// update moov size in newData
	// moov size field is at moov.Offset, 4 bytes (assuming normal size < 4GB)
	sizeOff := moov.Offset
	binary.BigEndian.PutUint32(newData[sizeOff:sizeOff+4], uint32(moov.Size+uint64(len(udtaBox))))
	return os.WriteFile(dstPath, newData, 0o644)
}
