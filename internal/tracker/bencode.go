package tracker

import (
	"errors"
	"fmt"
	"strconv"
)

// ScrapeEntry holds the scrape data for a single info hash.
type ScrapeEntry struct {
	Complete   int // seeders
	Incomplete int // leechers
}

// decodeScrapeResponse parses a bencoded scrape response.
// Expected format: d5:filesd20:<hash>d8:completei<n>e10:incompletei<n>eeee
func decodeScrapeResponse(data []byte) (map[[20]byte]ScrapeEntry, error) {
	result := make(map[[20]byte]ScrapeEntry)

	d, _, err := decodeDict(data)
	if err != nil {
		return nil, fmt.Errorf("decoding outer dict: %w", err)
	}

	filesRaw, ok := d["files"]
	if !ok {
		return result, nil
	}

	filesBytes, ok := filesRaw.([]byte)
	if !ok {
		return nil, errors.New("files value is not raw bytes")
	}

	// Parse the files dict: keys are 20-byte info hashes, values are dicts
	pos := 0
	if pos >= len(filesBytes) || filesBytes[pos] != 'd' {
		return nil, errors.New("files is not a dict")
	}
	pos++

	for pos < len(filesBytes) && filesBytes[pos] != 'e' {
		// Read string key (info hash)
		keyBytes, newPos, err := decodeString(filesBytes, pos)
		if err != nil {
			return nil, fmt.Errorf("decoding info hash key: %w", err)
		}
		pos = newPos

		if len(keyBytes) != 20 {
			// Skip this entry's value
			newPos, err = skipValue(filesBytes, pos)
			if err != nil {
				return nil, fmt.Errorf("skipping non-20-byte key value: %w", err)
			}
			pos = newPos
			continue
		}

		var hash [20]byte
		copy(hash[:], keyBytes)

		// Parse the inner dict for complete/incomplete
		if pos >= len(filesBytes) || filesBytes[pos] != 'd' {
			return nil, errors.New("expected dict for hash entry")
		}
		pos++

		entry := ScrapeEntry{}
		for pos < len(filesBytes) && filesBytes[pos] != 'e' {
			fieldKey, newPos, err := decodeString(filesBytes, pos)
			if err != nil {
				return nil, fmt.Errorf("decoding field key: %w", err)
			}
			pos = newPos

			switch string(fieldKey) {
			case "complete":
				n, newPos, err := decodeInt(filesBytes, pos)
				if err != nil {
					return nil, fmt.Errorf("decoding complete: %w", err)
				}
				entry.Complete = n
				pos = newPos
			case "incomplete":
				n, newPos, err := decodeInt(filesBytes, pos)
				if err != nil {
					return nil, fmt.Errorf("decoding incomplete: %w", err)
				}
				entry.Incomplete = n
				pos = newPos
			default:
				newPos, err := skipValue(filesBytes, pos)
				if err != nil {
					return nil, fmt.Errorf("skipping field value: %w", err)
				}
				pos = newPos
			}
		}
		if pos < len(filesBytes) {
			pos++ // skip 'e' closing inner dict
		}

		result[hash] = entry
	}

	return result, nil
}

// decodeDict decodes a bencoded dict at the top level, returning string keys
// mapped to raw byte slices (for nested structures) or decoded values.
func decodeDict(data []byte) (map[string]interface{}, []byte, error) {
	if len(data) == 0 || data[0] != 'd' {
		return nil, nil, fmt.Errorf("expected 'd', got %q", string(data[:1]))
	}

	result := make(map[string]interface{})
	pos := 1

	for pos < len(data) && data[pos] != 'e' {
		key, newPos, err := decodeString(data, pos)
		if err != nil {
			return nil, nil, fmt.Errorf("decoding dict key: %w", err)
		}
		pos = newPos

		keyStr := string(key)
		startPos := pos
		endPos, err := skipValue(data, pos)
		if err != nil {
			return nil, nil, fmt.Errorf("skipping dict value for %q: %w", keyStr, err)
		}
		result[keyStr] = data[startPos:endPos]
		pos = endPos
	}

	if pos < len(data) {
		pos++ // skip 'e'
	}

	return result, data[pos:], nil
}

func decodeString(data []byte, pos int) ([]byte, int, error) {
	colonIdx := -1
	for i := pos; i < len(data); i++ {
		if data[i] == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return nil, 0, fmt.Errorf("no colon found for string at pos %d", pos)
	}

	length, err := strconv.Atoi(string(data[pos:colonIdx]))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid string length at pos %d: %w", pos, err)
	}

	start := colonIdx + 1
	end := start + length
	if end > len(data) {
		return nil, 0, fmt.Errorf("string extends past data at pos %d", pos)
	}

	return data[start:end], end, nil
}

func decodeInt(data []byte, pos int) (int, int, error) {
	if pos >= len(data) || data[pos] != 'i' {
		return 0, 0, fmt.Errorf("expected 'i' at pos %d", pos)
	}

	endIdx := -1
	for i := pos + 1; i < len(data); i++ {
		if data[i] == 'e' {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return 0, 0, fmt.Errorf("no 'e' found for int at pos %d", pos)
	}

	n, err := strconv.Atoi(string(data[pos+1 : endIdx]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid int at pos %d: %w", pos, err)
	}

	return n, endIdx + 1, nil
}

// skipValue advances past one bencoded value starting at pos.
// Returns the new position after the value.
func skipValue(data []byte, pos int) (int, error) {
	if pos >= len(data) {
		return 0, fmt.Errorf("unexpected end of data at pos %d", pos)
	}

	switch {
	case data[pos] == 'i': // integer
		_, newPos, err := decodeInt(data, pos)
		return newPos, err
	case data[pos] == 'l': // list
		pos++
		for pos < len(data) && data[pos] != 'e' {
			newPos, err := skipValue(data, pos)
			if err != nil {
				return 0, err
			}
			pos = newPos
		}
		if pos < len(data) {
			pos++
		}
		return pos, nil
	case data[pos] == 'd': // dict
		pos++
		for pos < len(data) && data[pos] != 'e' {
			// skip key
			_, newPos, err := decodeString(data, pos)
			if err != nil {
				return 0, err
			}
			pos = newPos
			// skip value
			newPos, err = skipValue(data, pos)
			if err != nil {
				return 0, err
			}
			pos = newPos
		}
		if pos < len(data) {
			pos++
		}
		return pos, nil
	case data[pos] >= '0' && data[pos] <= '9': // string
		_, newPos, err := decodeString(data, pos)
		return newPos, err
	default:
		return 0, fmt.Errorf("unexpected byte %q at pos %d", data[pos], pos)
	}
}
