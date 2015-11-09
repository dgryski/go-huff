package huff

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"

	"github.com/dgryski/go-bitstream"
)

func TestRoundtrip(t *testing.T) {

	data, err := ioutil.ReadFile("/usr/share/dict/words")
	if err != nil {
		t.Skip("unable to open words file")
	}

	counts := make([]int, 256)

	for _, v := range data {
		counts[v]++
	}

	e := NewEncoder(counts)

	t.Log(e)

	var b bytes.Buffer

	w := e.Writer(&b)

	for _, v := range data {
		w.WriteSymbol(uint32(v))
	}
	w.WriteSymbol(EOF)

	compressed := b.Bytes()

	t.Logf("%d -> %d\n", len(data), len(compressed))

	cbb := e.CodebookBytes()

	d, err := NewDecoder(cbb)
	if err != nil {
		t.Fatalf("error roundtripping codebook")
	}

	br := bitstream.NewReader(bytes.NewReader(compressed))

	var uncompressed []byte

	for {
		b, err := d.ReadSymbol(br)
		if b == EOF || err == io.EOF {
			break
		}
		uncompressed = append(uncompressed, byte(b))
		if err != nil {
			t.Errorf("err = %+v\n", err)
			break
		}
	}

	if !bytes.Equal(data, uncompressed) {
		t.Errorf("bytes compare found mismatch")
	}
}
