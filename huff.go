// Package huff is a simple huffman encoder/decoder
package huff

import (
	"container/heap"
	"errors"
	"io"

	"github.com/dgryski/go-bitstream"
)

var EOF uint32 = 0xffffffff

type symbol struct {
	s    uint32
	Code uint32
	Len  int
}

type Encoder struct {
	eof uint32
	m   []symbol
}

type node struct {
	weight int
	child  [2]*node
	leaf   bool
	sym    uint32
}

type nodes []node

func (n nodes) Len() int            { return len(n) }
func (n nodes) Swap(i, j int)       { n[i], n[j] = n[j], n[i] }
func (n nodes) Less(i, j int) bool  { return n[i].weight < n[j].weight }
func (n *nodes) Push(x interface{}) { *n = append(*n, x.(node)) }

func (n *nodes) Pop() interface{} {
	old := *n
	l := len(old)
	x := old[l-1]
	*n = old[0 : l-1]
	return x
}

func NewEncoder(counts []int) *Encoder {
	var n nodes

	for i, v := range counts {
		if v != 0 {
			heap.Push(&n, node{weight: v, leaf: true, sym: uint32(i)})
		}
	}

	// one more for EOF
	eof := uint32(len(counts))
	heap.Push(&n, node{weight: 0, leaf: true, sym: eof})

	for n.Len() > 1 {
		n1 := heap.Pop(&n).(node)
		n2 := heap.Pop(&n).(node)
		heap.Push(&n, node{weight: n1.weight + n2.weight, child: [2]*node{&n2, &n1}})
	}

	m := make([]symbol, eof+1)

	walk(&n[0], 0, 0, m)

	return &Encoder{eof: eof, m: m}
}

func walk(n *node, code uint32, depth int, m []symbol) {

	if n.leaf {
		m[n.sym] = symbol{s: n.sym, Code: code, Len: depth}
		return
	}

	walk(n.child[0], code<<1, depth+1, m)
	walk(n.child[1], (code<<1)|1, depth+1, m)
}

func (e *Encoder) Writer(w io.Writer) *Writer {
	return &Writer{e: e, BitWriter: bitstream.NewWriter(w)}
}

type Writer struct {
	e *Encoder
	*bitstream.BitWriter
}

var ErrUnknownSymbol = errors.New("huff: unknown symbol")

func (w *Writer) WriteSymbol(s uint32) error {

	if s == EOF {
		s = w.e.eof
	}

	if s > w.e.eof {
		return ErrUnknownSymbol
	}

	sym := w.e.m[s]

	w.BitWriter.WriteBits(uint64(sym.Code), sym.Len)

	return nil
}

type Decoder struct {
	*bitstream.BitReader
	m      []symbol
	codes  map[uint32]uint32
	eof    uint32
	maxlen int
}

func (e *Encoder) Decoder(r io.Reader) *Decoder {
	codes := make(map[uint32]uint32)

	var max int

	for i, sym := range e.m {
		codes[sym.Code] = uint32(i)
		if sym.Len > max {
			max = sym.Len
		}
	}

	return &Decoder{
		BitReader: bitstream.NewReader(r),
		m:         e.m,
		codes:     codes,
		eof:       e.eof,
		maxlen:    max,
	}
}

func (d *Decoder) ReadSymbol() (uint32, error) {
	var c uint32

	for i := 0; i < d.maxlen; i++ {
		b, err := d.ReadBit()
		if err != nil {
			return 0, err
		}

		c <<= 1
		if b {
			c |= 1
		}

		if s, ok := d.codes[c]; ok {
			if s == d.eof {
				s = EOF
			}
			return s, nil
		}
	}

	return 0, ErrUnknownSymbol
}
