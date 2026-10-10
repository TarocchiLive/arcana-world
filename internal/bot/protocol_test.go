package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

type protocolHandlerFunc func(context.Context, string, json.RawMessage) (any, *Error)

func (f protocolHandlerFunc) Call(ctx context.Context, method string, params json.RawMessage) (any, *Error) {
	return f(ctx, method, params)
}

type protocolOutput struct {
	bytes.Buffer
}

func (*protocolOutput) Close() error { return nil }

func TestProtocolExactIDsAndMixedBatch(t *testing.T) {
	calls := 0
	handler := protocolHandlerFunc(func(_ context.Context, method string, _ json.RawMessage) (any, *Error) {
		calls++
		if method == "missing" {
			return nil, &Error{Code: -32601, Message: "Method not found"}
		}
		return nil, nil
	})
	input := "[" +
		`{"jsonrpc":"2.0","method":"ok","id":1234567890123456789012345678901234567890},` +
		`{"jsonrpc":"2.0","method":"ok","id":"<\u0061>"},` +
		`{"jsonrpc":"2.0","method":"missing"},` +
		`{"jsonrpc":"2.0","method":"ok","params":null},` +
		`{"jsonrpc":"2.0","method":"ok","params":false,"id":7},` +
		`42,{"jsonrpc":"2.0","method":"ok","id":null}]` + "\r\n" +
		`{"jsonrpc":"2.0","method":"ok","id":1e9999}`
	var output protocolOutput
	if err := Serve(context.Background(), io.NopCloser(strings.NewReader(input)), &output, handler); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	var batch []map[string]json.RawMessage
	if err := decoder.Decode(&batch); err != nil || len(batch) != 5 {
		t.Fatalf("invalid batch response: %s (%v)", output.String(), err)
	}
	if string(batch[0]["id"]) != "1234567890123456789012345678901234567890" {
		t.Fatalf("large ID lost precision: %s", batch[0]["id"])
	}
	var stringID string
	if err := json.Unmarshal(batch[1]["id"], &stringID); err != nil || stringID != "<a>" {
		t.Fatalf("string ID changed: %s", batch[1]["id"])
	}
	for _, index := range []int{0, 1, 4} {
		if string(batch[index]["result"]) != "null" || batch[index]["error"] != nil {
			t.Fatalf("invalid success response: %s", output.String())
		}
	}
	for index, code := range map[int]int{2: -32602, 3: -32600} {
		var rpcErr Error
		if err := json.Unmarshal(batch[index]["error"], &rpcErr); err != nil || rpcErr.Code != code || batch[index]["result"] != nil {
			t.Fatalf("invalid error response: %s", output.String())
		}
	}
	if string(batch[2]["id"]) != "7" || string(batch[3]["id"]) != "null" || string(batch[4]["id"]) != "null" {
		t.Fatalf("response IDs changed: %s", output.String())
	}
	var final map[string]json.RawMessage
	if err := decoder.Decode(&final); err != nil || string(final["id"]) != "1e9999" || string(final["result"]) != "null" {
		t.Fatalf("final frame or exponent ID changed: %s (%v)", output.String(), err)
	}
	if err := decoder.Decode(&final); err != io.EOF {
		t.Fatalf("unexpected extra response: %v", err)
	}
	if calls != 5 {
		t.Fatalf("handler calls = %d, want 5", calls)
	}
}

func TestProtocolRejectsBeforeDispatch(t *testing.T) {
	request := `{"jsonrpc":"2.0","method":"send_message","id":1}`
	cases := []struct {
		name    string
		input   string
		code    string
		wantErr bool
	}{
		{"empty batch", "[]\n", `"code":-32600`, false},
		{"large batch", "[" + strings.Repeat(request+",", maxBatchSize) + request + "]\n", `"code":-32600`, false},
		{"partial EOF", `{"jsonrpc":"2.0"`, `"code":-32700`, false},
		{"blank frame", "\n", `"code":-32700`, false},
		{"oversized frame", strings.Repeat(" ", maxFrameBytes+1) + "\n", "", true},
		{"null method", `{"jsonrpc":"2.0","method":null,"id":1}`, `"code":-32600`, false},
		{"invalid ID", `{"jsonrpc":"2.0","method":"send_message","id":true}`, `"code":-32600`, false},
		{"invalid UTF-8", "{\"jsonrpc\":\"2.0\",\"method\":\"send_message\",\"id\":\"\xff\"}", `"code":-32700`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			handler := protocolHandlerFunc(func(context.Context, string, json.RawMessage) (any, *Error) {
				calls++
				return nil, nil
			})
			var output protocolOutput
			err := Serve(context.Background(), io.NopCloser(strings.NewReader(tc.input)), &output, handler)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Serve error = %v, wantErr %v", err, tc.wantErr)
			}
			if calls != 0 {
				t.Fatalf("rejected input invoked handler %d times", calls)
			}
			if tc.code != "" && !strings.Contains(output.String(), tc.code) {
				t.Fatalf("missing error %s: %s", tc.code, output.String())
			}
			if tc.wantErr && output.Len() != 0 {
				t.Fatalf("oversized input produced output: %s", output.String())
			}
		})
	}
}

type protocolBlockingOutput struct {
	*io.PipeWriter
	started chan struct{}
}

func (w *protocolBlockingOutput) Write(p []byte) (int, error) {
	close(w.started)
	return w.PipeWriter.Write(p)
}

func TestProtocolCancellationUnblocksStreams(t *testing.T) {
	for _, writing := range []bool{false, true} {
		name := "reading"
		if writing {
			name = "writing"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input, inputWriter := io.Pipe()
			defer input.Close()
			defer inputWriter.Close()
			outputReader, output := io.Pipe()
			defer outputReader.Close()
			defer output.Close()
			blockingOutput := &protocolBlockingOutput{PipeWriter: output, started: make(chan struct{})}
			handler := protocolHandlerFunc(func(context.Context, string, json.RawMessage) (any, *Error) {
				return nil, nil
			})
			finished := make(chan error, 1)
			go func() {
				finished <- Serve(ctx, input, blockingOutput, handler)
			}()
			if writing {
				if _, err := io.WriteString(inputWriter, "{\"jsonrpc\":\"2.0\",\"method\":\"ok\",\"id\":1}\n"); err != nil {
					t.Fatal(err)
				}
				select {
				case <-blockingOutput.started:
				case <-time.After(2 * time.Second):
					t.Fatal("response write did not start")
				}
			}
			cancel()
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancellation left Serve blocked")
			}
		})
	}
}
