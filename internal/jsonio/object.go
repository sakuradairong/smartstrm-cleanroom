package jsonio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeObject accepts exactly one bounded JSON object. Reading the extra byte
// prevents accepting a valid prefix followed by an oversized trailing body.
func DecodeObject(reader io.Reader, target any, maximum int64) error {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}
	if int64(len(data)) > maximum {
		return fmt.Errorf("JSON body exceeds %d bytes", maximum)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("body must contain one JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return err
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("body must contain one JSON object")
	}
	return nil
}
