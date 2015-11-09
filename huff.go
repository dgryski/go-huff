// Package huff is a simple huffman encoder/decoder
package huff

import (
	"bytes"
	"container/heap"
	"encoding/binary"
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

type codebook []symbol

func (c codebook) calculateCodes() (symptrs, []uint32) {
	var sptrs symptrs
	for i := range c {
		if c[i].Len != 0 {
			sptrs = append(sptrs, &c[i])
		}
	}
	sort.Sort(sptrs)

	var code uint32
	numl := make([]uint32, sptrs[len(sptrs)-1].Len+1)
	prevlen := -1
	for i := range sptrs {
		if sptrs[i].Len > prevlen {
			code <<= uint(sptrs[i].Len - prevlen)
			prevlen = sptrs[i].Len
		}
		numl[sptrs[i].Len]++
		sptrs[i].Code = code
		code++
	}

	return sptrs, numl
}

func (c codebook) MarshalBinary() ([]byte, error) {
	var b []byte

	var vbuf [binary.MaxVarintLen32]byte

	l := binary.PutVarint(vbuf[:], int64(len(c)))
	b = append(b, vbuf[:l]...)

	for i := range c {
		l := binary.PutVarint(vbuf[:], int64(c[i].Len))
		b = append(b, vbuf[:l]...)
	}

	return b, nil
}

var ErrInvalidCodebook = errors.New("huff: invalid codebook")

func (c *codebook) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)

	l, err := binary.ReadVarint(r)
	if err != nil {
		return ErrInvalidCodebook
	}

	// TODO(dgryski): sanity check `l`

	*c = make(codebook, l)

	for i := uint32(0); i < uint32(l); i++ {
		clen, err := binary.ReadVarint(r)
		if err != nil {
			return ErrInvalidCodebook
		}
		(*c)[i] = symbol{s: i, Len: int(clen)}
	}

	return nil
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

type Encoder struct {
	eof  uint32
	m    codebook
	sym  symptrs
	numl []uint32
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

	m := make(codebook, eof+1)
	walk(&n[0], 0, m)

	sptrs, numl := m.calculateCodes()

	return &Encoder{eof: eof, m: m, sym: sptrs, numl: numl}
}

func walk(n *node, depth int, m codebook) {

	if n.leaf {
		m[n.sym] = symbol{s: n.sym, Len: depth}
		return
	}

	walk(n.child[0], depth+1, m)
	walk(n.child[1], depth+1, m)
}

func (e *Encoder) SymbolLen(s uint32) int {

	if s == EOF {
		s = e.eof
	}

	if s < 0 || s >= uint32(len(e.m)) {
		return 0
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
	eof  uint32
	numl []uint32
	sym  []*symbol
}

func (e *Encoder) Decoder() *Decoder {
	return &Decoder{
		eof:  e.eof,
		numl: e.numl,
		sym:  e.sym,
	}
}

func (d *Decoder) ReadSymbol(br *bitstream.BitReader) (uint32, error) {
	var offset uint32
	var code uint32

	for i := 0; i < len(d.numl); i++ {
		b, err := br.ReadBit()
		if err != nil {
			return 0, err
		}

		code <<= 1
		if b {
			code |= 1
		}

		offset += d.numl[i]
		first := d.sym[offset].Code

		if code-first < d.numl[i+1] {
			s := d.sym[code-first+offset].s
			if s == d.eof {
				s = EOF
			}
			return s, nil
		}
	}

	return 0, ErrUnknownSymbol
}
