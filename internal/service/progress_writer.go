package service

import (
	"fmt"
	"io"
	"os"
	"time"
)

type ProgressWriter struct {
	w     io.Writer
	n     int64
	total int64
	last  time.Time
}

func NewProgressWriter(w io.Writer, total int64) *ProgressWriter {
	return &ProgressWriter{w: w, total: total}
}

func (p *ProgressWriter) Update() {
	end := " ..."
	if p.n == p.total {
		end = ""
	}
	fmt.Fprintf(os.Stderr, "Downloaded %5.1f%% (%*d / %d bytes)%s\n",
		(100.0*float64(p.n))/float64(p.total),
		ndigits(p.total), p.n, p.total, end)
}

func ndigits(i int64) int {
	var n int
	for ; i != 0; i /= 10 {
		n++
	}
	return n
}

func (p *ProgressWriter) Write(buf []byte) (n int, err error) {
	n, err = p.w.Write(buf)
	p.n += int64(n)
	if now := time.Now(); now.Unix() != p.last.Unix() {
		p.Update()
		p.last = now
	}
	return
}
