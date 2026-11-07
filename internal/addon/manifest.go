package addon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const maxManifestSize = 1 << 20

// Parse validates a single JSON manifest for local imports and remote updates.
func Parse(data []byte) (Definition, error) {
	if len(data) > maxManifestSize {
		return Definition{}, fmt.Errorf("add-on manifest exceeds 1 MB")
	}
	var d Definition
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Definition{}, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return Definition{}, fmt.Errorf("expected one JSON object")
	}
	if err := d.Validate(); err != nil {
		return Definition{}, err
	}
	return d, nil
}
