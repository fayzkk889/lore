package engine

import (
	"bufio"
	"io"
	"strings"
)

// sseFrame is one server-sent event: an optional event name and a data line.
type sseFrame struct {
	event string
	data  string
}

// readSSE reads SSE frames from r and invokes fn for each frame that has a
// data payload. It returns the first read error (nil on clean EOF).
func readSSE(r io.Reader, fn func(sseFrame) bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024) // generous: file contents stream as JSON deltas

	var ev, data string
	flush := func() bool {
		if data == "" {
			ev = ""
			return true
		}
		f := sseFrame{event: ev, data: data}
		ev, data = "", ""
		return fn(f)
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if !flush() {
				return nil
			}
		case strings.HasPrefix(line, "event:"):
			ev = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			d := strings.TrimSpace(line[len("data:"):])
			if data != "" {
				data += "\n"
			}
			data += d
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	flush()
	return nil
}
