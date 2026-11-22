package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"testing"

	"roundos/sdk/rui/spec"
)

func fixture() *File {
	return &File{Format: Format, Document: spec.Document{Schema: spec.Version, Application: spec.Application{ID: "org.roundos.timer"}, Screens: []spec.Screen{{ID: "root"}}}, Manifest: spec.Manifest{Bindings: map[string]spec.BindingType{}}, SourceSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", SourceMap: map[string]spec.Pos{}}
}
func TestRoundtripAndDeterminism(t *testing.T) {
	f := fixture()
	b, e := Encode(f)
	if e != nil {
		t.Fatal(e)
	}
	b2, e := Encode(f)
	if e != nil || !bytes.Equal(b, b2) {
		t.Fatalf("not deterministic: %v", e)
	}
	decoded, e := Decode(b)
	if e != nil || decoded.Format != Format || decoded.Document.Application.ID != "org.roundos.timer" {
		t.Fatalf("decode %v %+v", e, decoded)
	}
}
func TestRejectCorruptionAndTruncation(t *testing.T) {
	b, _ := Encode(fixture())
	trials := [][]byte{nil, b[:1], b[:len(b)-1], append(append([]byte(nil), b...), 0), append([]byte("BAD!"), b[4:]...)}
	mutated := append([]byte(nil), b...)
	mutated[9] ^= 0xff
	trials = append(trials, mutated)
	tooLarge := append([]byte(nil), b...)
	binary.BigEndian.PutUint32(tooLarge[4:8], MaxPayload+1)
	trials = append(trials, tooLarge)
	for i, x := range trials {
		if _, err := Decode(x); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}
func TestRejectUnknownFieldDespiteMatchingChecksum(t *testing.T) {
	f := fixture()
	b, _ := json.Marshal(f)
	b = bytes.Replace(b, []byte(`"format":"`), []byte(`"unknown":5,"format":"`), 1)
	out := make([]byte, 8+len(b)+32)
	copy(out[:4], "RUI2")
	binary.BigEndian.PutUint32(out[4:8], uint32(len(b)))
	copy(out[8:], b)
	sum := sha256.Sum256(b)
	copy(out[8+len(b):], sum[:])
	if _, err := Decode(out); err == nil {
		t.Fatal("accepted unknown compiled field")
	}
}
