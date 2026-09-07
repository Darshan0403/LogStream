package parser

import "testing"

func TestJSONParser_SingleAndBatch(t *testing.T) {
	p := &JSONParser{}

	e, err := p.Parse([]byte(`{"level":"error","service":"api","message":"boom"}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.Message != "boom" || e.Service != "api" {
		t.Fatalf("bad parse: %+v", e)
	}

	// defaults
	d, _ := p.Parse([]byte(`{"message":"x"}`))
	if d.Level != "INFO" || d.Timestamp.IsZero() || d.Metadata == nil {
		t.Fatalf("defaults not applied: %+v", d)
	}

	batch, err := p.ParseBatch([]byte(`[{"message":"a"},{"message":"b"}]`))
	if err != nil || len(batch) != 2 {
		t.Fatalf("batch: %v %d", err, len(batch))
	}
}

func TestTextParser_Formats(t *testing.T) {
	p := &TextParser{DefaultService: "svc"}
	cases := []struct {
		line      string
		wantLevel string
	}{
		{"192.168.65.1 - - [07/Jun/2026:07:36:12 +0000] \"GET / HTTP/1.1\" 500 0", "ERROR"},
		{"192.168.65.1 - - [07/Jun/2026:07:36:12 +0000] \"GET / HTTP/1.1\" 404 0", "WARN"},
		{"ERROR:     something failed", "ERROR"},
		{"2026-06-07 07:35:25.689 UTC [27] LOG:  checkpoint starting", "INFO"},
		{"a totally freeform line with no level", "INFO"},
		{"something WARNING happened", "WARN"},
	}
	for _, c := range cases {
		e, err := p.Parse([]byte(c.line))
		if err != nil {
			t.Fatalf("%q: %v", c.line, err)
		}
		if e.Level != c.wantLevel {
			t.Errorf("%q: level = %q, want %q", c.line, e.Level, c.wantLevel)
		}
		if e.Service != "svc" && e.Service == "" {
			t.Errorf("%q: service not defaulted", c.line)
		}
	}

	// Text parser never returns an error and never drops a non-empty line.
	if b, err := p.ParseBatch([]byte("   ")); err != nil || len(b) != 0 {
		t.Fatalf("blank line: %v %d", err, len(b))
	}
}

func TestDockerParser(t *testing.T) {
	p := NewDockerParser("fallback")

	// flexible JSON with msg/time aliases + extra field -> metadata
	e, err := p.Parse([]byte(`{"msg":"started","level":"warning","time":"2026-06-07T07:36:47Z","port":8080}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.Message != "started" || e.Level != "WARN" {
		t.Fatalf("bad: %+v", e)
	}
	if e.Metadata["port"] == nil {
		t.Fatalf("extra field not captured in metadata: %+v", e.Metadata)
	}

	// docker envelope wrapping a plain line
	env, err := p.Parse([]byte(`{"log":"plain stdout line\n","stream":"stdout","time":"2026-06-07T07:36:47Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	if env.Message != "plain stdout line" {
		t.Fatalf("envelope not unwrapped: %q", env.Message)
	}

	// non-JSON falls through to text parser and still succeeds
	txt, err := p.Parse([]byte("just some text"))
	if err != nil || txt.Message == "" {
		t.Fatalf("text fallthrough: %v %+v", err, txt)
	}
}

func TestNormaliseLevel(t *testing.T) {
	for in, want := range map[string]string{
		"warning": "WARN", "CRITICAL": "ERROR", "panic": "FATAL",
		"trace": "DEBUG", "notice": "INFO", "weird": "INFO",
	} {
		if got := normaliseLevel(in); got != want {
			t.Errorf("normaliseLevel(%q) = %q, want %q", in, got, want)
		}
	}
}
