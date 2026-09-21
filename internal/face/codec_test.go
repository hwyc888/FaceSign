package face

import "testing"

func TestCodecRoundTrip(t *testing.T) {
	in := []float32{-1.5, 0, 0.25, 99.5}
	out, err := Decode(Encode(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("got %d values, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("value %d: got %v want %v", i, out[i], in[i])
		}
	}
}
