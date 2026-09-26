package trace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceRedactsUnsafeIDs(t *testing.T) {
	for _, test := range []struct {
		name, operationID, want string
	}{
		{name: "uuid", operationID: "550e8400-e29b-41d4-a716-446655440000", want: "550e8400-e29b-41d4-a716-446655440000"},
		{name: "path", operationID: "/private/secret/token", want: "[redacted]"},
		{name: "newline", operationID: "token\nsecret", want: "[redacted]"},
		{name: "too long", operationID: strings.Repeat("x", 129), want: "[redacted]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()

			writer, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}

			Record(WithWriter(context.Background(), writer), Entry{
				Phase: "binding_entry", OperationID: test.operationID,
			})

			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			data, err := os.ReadFile(filepath.Join(dir, "trace.jsonl"))
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(string(data), `"operationId":"`+test.want+`"`) ||
				strings.Contains(string(data), test.operationID) && test.operationID != test.want {
				t.Fatalf("trace = %s", data)
			}
		})
	}
}
