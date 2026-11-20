// Package bundle defines the deterministic, size-bounded RUI v1 production
// artifact format. It is public so independent SDK applications can embed and
// decode compiled layouts without importing any RoundOS internal packages.
// Decoding authenticates file integrity; it does NOT authorize application IDs
// or execute bindings/handlers. M3 must check the trusted application identity.
package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"roundos/sdk/rui/spec"
)

const (
	Format     = "roundos.rui.bundle/v1"
	MaxPayload = 512 << 10
	header     = "RUI1"
	prefixSize = 8
	digestSize = 32
)

// File is a pure-data validated compiler product. SourceMap is relative to the
// RUI YAML file, never an absolute host path. The document additionally keeps
// its AST node positions, for diagnostics and development preview.
type File struct {
	Format       string              `json:"format"`
	Document     spec.Document       `json:"document"`
	Manifest     spec.Manifest       `json:"manifest"`
	SourceSHA256 string              `json:"sourceSHA256"`
	SourceMap    map[string]spec.Pos `json:"sourceMap"`
}

// Encode produces: magic(4), big-endian JSON length(4), canonical JSON,
// SHA256(JSON)(32). encoding/json sorts map keys; no timestamps or paths occur.
func Encode(f *File) ([]byte, error) {
	if f == nil || f.Format != Format || f.Document.Schema != spec.Version || f.Document.Application.ID == "" || len(f.Document.Screens) == 0 || f.Manifest.Bindings == nil || len(f.SourceSHA256) != 64 || f.SourceMap == nil {
		return nil, errors.New("incomplete RUI bundle")
	}
	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxPayload {
		return nil, fmt.Errorf("compiled bundle exceeds %d bytes", MaxPayload)
	}
	out := make([]byte, prefixSize+len(b)+digestSize)
	copy(out[:4], header)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(b)))
	copy(out[prefixSize:], b)
	sum := sha256.Sum256(b)
	copy(out[prefixSize+len(b):], sum[:])
	return out, nil
}

// Decode rejects truncated, trailing, corrupted, oversized or unknown-format
// data. Callers should additionally compare the embedded application ID to
// their authenticated app identity before publishing any view.
func Decode(data []byte) (*File, error) {
	if len(data) < prefixSize+digestSize || !bytes.Equal(data[:4], []byte(header)) {
		return nil, errors.New("invalid RUI bundle header")
	}
	n := int(binary.BigEndian.Uint32(data[4:8]))
	if n == 0 || n > MaxPayload || len(data) != prefixSize+n+digestSize {
		return nil, errors.New("invalid RUI bundle length")
	}
	b := data[prefixSize : prefixSize+n]
	sum := sha256.Sum256(b)
	if !bytes.Equal(sum[:], data[prefixSize+n:]) {
		return nil, errors.New("RUI bundle checksum mismatch")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("RUI bundle JSON: %w", err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing RUI bundle JSON")
	}
	if f.Format != Format || f.Document.Schema != spec.Version || f.Document.Application.ID == "" || len(f.Document.Screens) == 0 || f.Manifest.Bindings == nil || f.SourceMap == nil || len(f.SourceSHA256) != 64 {
		return nil, errors.New("unsupported or incomplete RUI bundle")
	}
	return &f, nil
}
