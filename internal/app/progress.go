package app

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/spurintel/cli/internal/output"
)

// progressLine is the destination, palette, and label for a download progress
// bar. The command layer builds it (deciding the writer and color the same way
// emit does) and hands it to downloadFeed; nil means no progress is shown.
type progressLine struct {
	w     io.Writer
	s     output.Styles
	label string
}

// newProgressLine binds a progress bar to w, styled when color is true.
func newProgressLine(w io.Writer, color bool, label string) *progressLine {
	return &progressLine{w: w, s: output.NewStyles(w, color), label: label}
}

// progressReader wraps a reader and draws a single self-overwriting transfer
// line to its progressLine (stderr) as bytes flow through it. The download
// command uses it so a terminal shows how far a feed download has gotten,
// without ever writing to stdout — stdout stays a clean data channel.
//
// It is only constructed when progress should be shown (an interactive stderr,
// not --silent); when it should not, the command streams the body directly and
// no progressReader exists.
type progressReader struct {
	r         io.Reader
	line      *progressLine
	total     int64 // bytes expected, or -1 when the server did not send a length
	read      int64
	last      time.Time
	redraw    time.Duration
	drawWidth int // visible width of the last drawn line, for clean overwrites
}

// newProgressReader wraps r, drawing progress to line. total is the expected
// byte count (-1 when unknown, in which case a plain byte counter is shown
// instead of a bar).
func newProgressReader(r io.Reader, line *progressLine, total int64) *progressReader {
	return &progressReader{r: r, line: line, total: total, redraw: 100 * time.Millisecond}
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
	fmt.Fprintln(p.line.w)
}

const barWidth = 20

func (p *progressReader) draw() {
	s := p.line.s
	var line string
	if p.total > 0 {
		frac := min(1, float64(p.read)/float64(p.total))
		line = fmt.Sprintf("%s %s %s  %s / %s",
			s.Value.Render(p.line.label),
			bar(s, frac),
			s.Num.Render(fmt.Sprintf("%3.0f%%", frac*100)),
			output.HumanizeBytes(p.read),
			output.HumanizeBytes(p.total))
	} else {
		line = fmt.Sprintf("%s… %s", s.Value.Render(p.line.label), output.HumanizeBytes(p.read))
	}

	// Pad to the widest line drawn so far so a shrinking tail (1023.9 KiB →
	// 1.0 MiB) never leaves residue after the \r overwrite.
	if w := lipgloss.Width(line); w >= p.drawWidth {
		p.drawWidth = w
	} else {
		line += strings.Repeat(" ", p.drawWidth-w)
	}
	fmt.Fprintf(p.line.w, "\r%s", line)
}

// bar renders a fixed-width transfer bar: a filled accent run over a muted
// track, so progress reads at a glance without a TUI runtime.
func bar(s output.Styles, frac float64) string {
	filled := int(frac*barWidth + 0.5)
	return s.Frame.Render(strings.Repeat("█", filled)) +
		s.Empty.UnsetItalic().Render(strings.Repeat("░", barWidth-filled))
}
