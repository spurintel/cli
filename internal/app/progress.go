package app

import (
	"fmt"
	"io"
	"time"
)

// progressReader wraps a reader and draws a single self-overwriting transfer
// line to its destination (stderr) as bytes flow through it. The download
// command uses it so a terminal shows how far a feed download has gotten,
// without ever writing to stdout — stdout stays a clean data channel.
//
// It is only constructed when progress should be shown (an interactive stderr,
// not --silent); when it should not, the command streams the body directly and
// no progressReader exists.
type progressReader struct {
	r      io.Reader
	w      io.Writer
	label  string
	total  int64 // bytes expected, or -1 when the server did not send a length
	read   int64
	last   time.Time
	redraw time.Duration
}

// newProgressReader wraps r, drawing progress for label to w. total is the
// expected byte count (-1 when unknown, in which case no percentage is shown).
func newProgressReader(r io.Reader, w io.Writer, label string, total int64) *progressReader {
	return &progressReader{r: r, w: w, label: label, total: total, redraw: 100 * time.Millisecond}
}

// Read passes through to the underlying reader, accumulates the byte count, and
// redraws the line at most once per redraw interval (always on the final read,
// so the last state is never lost to throttling).
func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	now := time.Now()
	if err != nil || now.Sub(p.last) >= p.redraw {
		p.draw()
		p.last = now
	}
	return n, err
}

// Finish redraws the final total and terminates the line so subsequent output
// starts cleanly. Call it once the copy is complete.
func (p *progressReader) Finish() {
	p.draw()
	fmt.Fprintln(p.w)
}

func (p *progressReader) draw() {
	if p.total > 0 {
		pct := float64(p.read) / float64(p.total) * 100
		fmt.Fprintf(p.w, "\r%s… %s / %s (%.0f%%)", p.label, humanBytes(p.read), humanBytes(p.total), pct)
	} else {
		fmt.Fprintf(p.w, "\r%s… %s", p.label, humanBytes(p.read))
	}
}

// humanBytes formats a byte count with a binary (1024-based) unit suffix.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
