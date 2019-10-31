package volume

import (
	"bytes"
	"encoding/gob"
)

func serialize(v Volume) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func unserialize(in []byte) (*Volume, error) {
	var buf bytes.Buffer

	buf.Write(in)

	out := &Volume{}
	dec := gob.NewDecoder(&buf)
	if err := dec.Decode(out); err != nil {
		return nil, err
	}

	return out, nil
}
