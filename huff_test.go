package huff

import (
	"bytes"
	"testing"
)

func TestHuff(t *testing.T) {

	counts := []int{3, 1, 4, 1, 5, 9}

	e := NewEncoder(counts)

	var buf bytes.Buffer

	w := e.Writer(&buf)

	stream := []uint32{5, 3, 1, 1, 3}

	for i := range stream {
		w.WriteSymbol(stream[i])
		w.WriteBits(uint64(0xfff), int(stream[i]))
	}

	w.WriteSymbol(EOF)
	w.Flush(false)

	d := e.Decoder(bytes.NewReader(buf.Bytes()))

	var i int
	var foundEOF bool
	for {
		s, err := d.ReadSymbol()
		if err != nil {
			break
		}
		if s == EOF {
			foundEOF = true
			break
		}
		if i >= len(stream) {
			break
		}
		if s != stream[i] {
			t.Errorf("stream index %d = %d, want %d", i, s, stream[i])
		}
		b, _ := d.ReadBits(int(s))
		want := uint64(1<<s) - 1
		if b != want {
			t.Errorf("stream item %d = %x, want %x", i, b, want)
		}
		i++
	}

	if !foundEOF {
		t.Errorf("did not find expected EOF token")
	}
}
