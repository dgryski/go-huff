// Package huff is a simple huffman encoder/decoder
package huff

import (
	"container/heap"
	"errors"
	"io"
	"sort"

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

type symptrs []*symbol

func (s symptrs) Len() int      { return len(s) }
func (s symptrs) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s symptrs) Less(i, j int) bool {
	return s[i].Len < s[j].Len || s[i].Len == s[j].Len && s[i].s < s[j].s
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

	var sptrs symptrs

	m := make([]symbol, eof+1)

	walk(&n[0], 0, 0, m, &sptrs)

	sort.Sort(sptrs)

	var code uint32
	prevlen := -1
	for i := range sptrs {
		if sptrs[i].Len > prevlen {
			code <<= uint(sptrs[i].Len - prevlen)
			prevlen = sptrs[i].Len
		}
		sptrs[i].Code = code
		code++
	}

	return &Encoder{eof: eof, m: m}
}

func walk(n *node, code uint32, depth int, m []symbol, sptrs *symptrs) {

	if n.leaf {
		m[n.sym] = symbol{s: n.sym, Len: depth}
		*sptrs = append(*sptrs, &m[n.sym])
		return
	}

	walk(n.child[0], code<<1, depth+1, m, sptrs)
	walk(n.child[1], (code<<1)|1, depth+1, m, sptrs)
}

func (e *Encoder) SymbolLen(s uint32) int {

	if s == EOF {
		s = e.eof
	}

	return e.m[s].Len
}

func (e *Encoder) Writer(w io.Writer) *Writer {
	return &Writer{e: e, BitWriter: bitstream.NewWriter(w)}
}

type Writer struct {
	e *Encoder
	*bitstream.BitWriter
}

var ErrUnknownSymbol = errors.New("huff: unknown symbol")

func (w *Writer) WriteSymbol(s uint32) (int, error) {

	if s == EOF {
		s = w.e.eof
	}

	if s > w.e.eof {
		return 0, ErrUnknownSymbol
	}

	sym := w.e.m[s]

	w.BitWriter.WriteBits(uint64(sym.Code), sym.Len)

	return sym.Len, nil
}

type Decoder struct {
	*bitstream.BitReader
	m      []symbol
	codes  map[uint64]uint32
	eof    uint32
	maxlen int
}

func (e *Encoder) Decoder(r io.Reader) *Decoder {
	codes := make(map[uint64]uint32)

	var max int

	for i, sym := range e.m {
		l := uint64(sym.Len)<<56 + uint64(sym.Code)
		codes[l] = uint32(i)
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

		l := uint64(i+1)<<56 + uint64(c)
		if s, ok := d.codes[l]; ok {
			if s == d.eof {
				s = EOF
			}
			return s, nil
		}
	}

	return 0, ErrUnknownSymbol
}
