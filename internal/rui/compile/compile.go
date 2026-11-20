// Package compile converts validated RUI v1 YAML into deterministic portable
// bundles; it never evaluates application bindings or executes actions.
package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"roundos/internal/rui/parse"
	"roundos/internal/rui/validate"
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
)

func Compile(src []byte) ([]byte, *bundle.File, error) {
	d, err := parse.Parse(src)
	if err != nil {
		return nil, nil, err
	}
	return FromDocument(d, src)
}

func FromDocument(d *spec.Document, src []byte) ([]byte, *bundle.File, error) {
	manifest, err := validate.Validate(d)
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(src)
	f := &bundle.File{Format: bundle.Format, Document: *d, Manifest: *manifest, SourceSHA256: hex.EncodeToString(sum[:]), SourceMap: SourceMap(d)}
	bytes, err := bundle.Encode(f)
	if err != nil {
		return nil, nil, err
	}
	return bytes, f, nil
}

// Verify performs full semantic verification, including re-deriving the action
// and binding manifest so a checksum-valid but maliciously rewritten manifest
// cannot mislead tooling. M3 must still authorize application identity.
func Verify(data []byte) (*bundle.File, error) {
	f, err := bundle.Decode(data)
	if err != nil {
		return nil, err
	}
	manifest, err := validate.Validate(&f.Document)
	if err != nil {
		return nil, fmt.Errorf("invalid compiled document: %w", err)
	}
	if !equalManifest(*manifest, f.Manifest) {
		return nil, fmt.Errorf("compiled binding/action manifest does not match document")
	}
	sm := SourceMap(&f.Document)
	if len(sm) != len(f.SourceMap) {
		return nil, fmt.Errorf("compiled source map differs from document")
	}
	for k, pos := range sm {
		if other, exists := f.SourceMap[k]; !exists || other != pos {
			return nil, fmt.Errorf("compiled source map differs from document at %s", k)
		}
	}
	return f, nil
}
func equalManifest(a, b spec.Manifest) bool {
	if len(a.Bindings) != len(b.Bindings) || len(a.Actions) != len(b.Actions) {
		return false
	}
	for k, v := range a.Bindings {
		if b.Bindings[k] != v {
			return false
		}
	}
	for i, v := range a.Actions {
		if b.Actions[i] != v {
			return false
		}
	}
	return true
}

// SourceMap identifies YAML nodes using human-readable stable semantic paths.
func SourceMap(d *spec.Document) map[string]spec.Pos {
	sm := map[string]spec.Pos{"application": d.Application.Pos}
	for _, screen := range d.Screens {
		sk := "screen/" + screen.ID
		sm[sk] = screen.Pos
		for _, group := range screen.Groups {
			gk := sk + "/group/" + group.ID
			sm[gk] = group.Pos
			for _, widget := range group.Widgets {
				sm[gk+"/widget/"+widget.ID] = widget.Pos
			}
		}
	}
	for _, dialog := range d.Dialogs {
		sm["dialog/"+dialog.ID] = dialog.Pos
	}
	return sm
}
