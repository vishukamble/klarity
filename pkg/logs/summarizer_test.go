package logs

import (
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name     string
		logs     string
		wantContains string // non-empty: result must contain this substring
		wantExact    string // non-empty: result must equal this exactly
	}{
		// ── Empty ────────────────────────────────────────────────────────────────
		{
			name:      "empty string",
			logs:      "",
			wantExact: "",
		},

		// ── Java ─────────────────────────────────────────────────────────────────
		{
			name: "java single caused-by",
			logs: `2024-01-15 10:00:00.123 ERROR com.example.App - Startup failed
java.lang.RuntimeException: Failed to initialize context
	at com.example.App.main(App.java:42)
Caused by: java.sql.SQLException: Connection refused to host: db:5432
	at com.mysql.jdbc.ConnectionImpl.createNewIO(ConnectionImpl.java:2181)`,
			wantContains: "Caused by: java.sql.SQLException",
		},
		{
			name: "java multiple caused-by returns last",
			logs: `java.lang.RuntimeException: outer
	at Foo.bar(Foo.java:10)
Caused by: java.io.IOException: middle error
	at Foo.baz(Foo.java:20)
Caused by: java.net.ConnectException: Connection refused
	at Foo.qux(Foo.java:30)`,
			wantContains: "Caused by: java.net.ConnectException",
		},
		{
			name: "java caused-by with leading whitespace trimmed",
			logs: `Exception in thread "main" java.lang.Error
	at App.run(App.java:5)
	Caused by: java.lang.NullPointerException: cannot read field "id"`,
			wantContains: "Caused by: java.lang.NullPointerException",
		},

		// ── Python ───────────────────────────────────────────────────────────────
		{
			name: "python traceback single exception",
			logs: `Starting service...
Traceback (most recent call last):
  File "/app/main.py", line 42, in <module>
    db.connect()
  File "/app/db.py", line 17, in connect
    raise ConnectionError("could not connect to postgres")
ConnectionError: could not connect to postgres`,
			wantContains: "ConnectionError: could not connect to postgres",
		},
		{
			name: "python traceback last of multiple",
			logs: `[INFO] Starting app
Traceback (most recent call last):
  File "app.py", line 10, in run
    load_config()
ValueError: bad config
[INFO] Retrying
Traceback (most recent call last):
  File "app.py", line 22, in run
    connect_db()
  File "db.py", line 5, in connect_db
    raise RuntimeError("db unavailable")
RuntimeError: db unavailable`,
			wantContains: "RuntimeError: db unavailable",
		},
		{
			name: "python keyerror",
			logs: `Traceback (most recent call last):
  File "worker.py", line 8, in process
    val = data["key"]
KeyError: 'key'`,
			wantContains: "KeyError: 'key'",
		},

		// ── Go ───────────────────────────────────────────────────────────────────
		{
			name: "go panic",
			logs: `2024/01/15 10:00:01 Starting server on :8080
2024/01/15 10:00:05 handling request /api/v1/users
panic: runtime error: index out of range [3] with length 2

goroutine 1 [running]:
main.handleUsers(...)
	/app/main.go:55 +0x198`,
			wantContains: "panic: runtime error: index out of range",
		},
		{
			name: "go fatal error",
			logs: `runtime: out of memory: cannot allocate
fatal error: runtime: out of memory

runtime stack:
runtime.throw2({0x5a1b40?, 0x0?})
	/usr/local/go/src/runtime/panic.go:1023 +0x57`,
			wantContains: "fatal error: runtime: out of memory",
		},
		{
			name: "go panic takes priority over generic error",
			logs: `Error initializing config
panic: nil pointer dereference`,
			wantContains: "panic: nil pointer dereference",
		},

		// ── Generic fatal ─────────────────────────────────────────────────────────
		{
			name: "generic FATAL line",
			logs: `[2024-01-15 10:00:01] INFO  Server starting up
[2024-01-15 10:00:03] INFO  Connecting to database
[2024-01-15 10:00:04] FATAL Could not bind to port 8080: address already in use
[2024-01-15 10:00:04] INFO  Shutting down`,
			wantContains: "FATAL Could not bind to port 8080",
		},
		{
			name: "generic Exception line",
			logs: `[INFO] Application starting
[ERROR] Exception in thread: NullPointerException at line 42
[INFO] Exiting`,
			wantContains: "Exception in thread",
		},
		{
			name: "generic Error line (no other match)",
			logs: `Starting app
Error: configuration file not found at /etc/app/config.yaml`,
			wantContains: "Error: configuration file not found",
		},

		// ── Connection errors ────────────────────────────────────────────────────
		{
			name: "connection refused ECONNREFUSED",
			logs: `Connecting to redis:6379...
connect ECONNREFUSED 10.0.0.5:6379`,
			wantContains: "ECONNREFUSED",
		},
		{
			name: "dial tcp connection refused",
			logs: `time="2024-01-15T10:00:05Z" level=info msg="starting"
time="2024-01-15T10:00:06Z" level=error msg="dial tcp 10.96.0.10:443: connect: connection refused"`,
			wantContains: "dial tcp",
		},

		// ── Auth errors ───────────────────────────────────────────────────────────
		{
			name: "401 unauthorized",
			logs: `Fetching config from API
HTTP 401 Unauthorized: token expired`,
			wantContains: "401",
		},
		{
			name: "permission denied",
			logs: `[app] loading TLS cert
[app] permission denied: /etc/ssl/private/server.key`,
			wantContains: "permission denied",
		},

		// ── Fallback ──────────────────────────────────────────────────────────────
		{
			name: "fallback last non-empty line",
			logs: `normal log line one
normal log line two
the final message
`,
			wantContains: "the final message",
		},
		{
			name: "fallback strips trailing blank lines",
			logs: "line one\nline two\n\n\n",
			wantExact: "line two",
		},
		{
			name: "single line log",
			logs: "Server running on port 8080",
			wantExact: "Server running on port 8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.logs)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("got %q, want exact %q", got, tt.wantExact)
			}
			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("got %q, want it to contain %q", got, tt.wantContains)
			}
		})
	}
}

// TestSummarizePriorityOrder ensures higher-priority patterns win over lower ones.
func TestSummarizePriorityOrder(t *testing.T) {
	tests := []struct {
		name         string
		logs         string
		wantContains string
	}{
		{
			// Java "Caused by" should win over generic "Error"
			name: "java beats generic error",
			logs: `Error initializing application
java.lang.RuntimeException: startup failed
Caused by: java.io.IOException: disk full`,
			wantContains: "Caused by: java.io.IOException",
		},
		{
			// Go panic should beat generic "Error" lines that come before it
			name: "go panic beats earlier error line",
			logs: `Error: config warning (non-fatal)
panic: nil pointer dereference`,
			wantContains: "panic:",
		},
		{
			// Python traceback should beat generic "Error" if it has a traceback header
			name: "python beats generic error",
			logs: `Error loading module
Traceback (most recent call last):
  File "app.py", line 1, in main
    run()
ImportError: No module named 'mylib'`,
			wantContains: "ImportError:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.logs)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("got %q, want it to contain %q", got, tt.wantContains)
			}
		})
	}
}
