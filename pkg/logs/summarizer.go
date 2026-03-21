package logs

import (
	"strings"
)

// Summarize extracts a single-line root-cause string from raw pod log text.
//
// Priority order:
//  1. Java   — last "Caused by:" line
//  2. Python — line immediately after the last "Traceback (most recent call last):" block header
//     (i.e., the actual exception line at the end of the traceback)
//  3. Go     — first "panic:" or "fatal error:" line
//  4. Generic fatal — first line matching FATAL|PANIC|Exception|Error
//  5. Connection errors — ECONNREFUSED, "connection refused", "dial tcp"
//  6. Auth errors — 401, 403, "authentication failed", "permission denied"
//  7. Fallback — last non-empty line
func Summarize(logs string) string {
	if logs == "" {
		return ""
	}
	lines := splitLines(logs)

	// 1. Java — last "Caused by:" line
	if s := lastMatchingLine(lines, "Caused by:"); s != "" {
		return strings.TrimSpace(s)
	}

	// 2. Python — last line of the last traceback block.
	// A traceback ends with the exception line, which is the non-indented line
	// that follows indented lines after the "Traceback" header.
	if s := pythonException(lines); s != "" {
		return s
	}

	// 3. Go — first "panic:" or "fatal error:" line
	if s := firstMatchingLineCaseInsensitive(lines, "panic:", "fatal error:"); s != "" {
		return strings.TrimSpace(s)
	}

	// 4. Generic fatal keywords
	if s := firstMatchingLineCaseInsensitive(lines, "FATAL", "PANIC", "Exception", "Error"); s != "" {
		return strings.TrimSpace(s)
	}

	// 5. Connection errors
	if s := firstMatchingLineCaseInsensitive(lines, "ECONNREFUSED", "connection refused", "dial tcp"); s != "" {
		return strings.TrimSpace(s)
	}

	// 6. Auth errors
	if s := firstMatchingLineCaseInsensitive(lines, "401", "403", "authentication failed", "permission denied"); s != "" {
		return strings.TrimSpace(s)
	}

	// 7. Fallback — last non-empty line
	return lastNonEmpty(lines)
}

// splitLines splits text into lines, stripping trailing \r.
func splitLines(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		out = append(out, strings.TrimRight(l, "\r"))
	}
	return out
}

// lastMatchingLine returns the last line that contains substr (case-sensitive).
func lastMatchingLine(lines []string, substr string) string {
	last := ""
	for _, l := range lines {
		if strings.Contains(l, substr) {
			last = l
		}
	}
	return last
}

// firstMatchingLineCaseInsensitive returns the first line containing any of
// the given substrings (case-insensitive match).
func firstMatchingLineCaseInsensitive(lines []string, substrs ...string) string {
	for _, l := range lines {
		lower := strings.ToLower(l)
		for _, sub := range substrs {
			if strings.Contains(lower, strings.ToLower(sub)) {
				return l
			}
		}
	}
	return ""
}

// pythonException finds the exception line at the end of the last Python
// traceback block. A Python traceback looks like:
//
//	Traceback (most recent call last):
//	  File "...", line N, in ...
//	    code snippet
//	ExceptionType: message
//
// The exception line is the first non-indented, non-empty line after the last
// "Traceback (most recent call last):" header.
func pythonException(lines []string) string {
	lastTBIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "Traceback (most recent call last):") {
			lastTBIdx = i
		}
	}
	if lastTBIdx < 0 {
		return ""
	}
	// Walk forward from the traceback header, skip indented lines,
	// and return the first non-indented non-empty line.
	for i := lastTBIdx + 1; i < len(lines); i++ {
		l := lines[i]
		if l == "" {
			continue
		}
		// Indented lines are part of the stack frames.
		if l[0] == ' ' || l[0] == '\t' {
			continue
		}
		return strings.TrimSpace(l)
	}
	return ""
}

// lastNonEmpty returns the last non-empty line.
func lastNonEmpty(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
