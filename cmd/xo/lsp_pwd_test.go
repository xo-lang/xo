package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLSPStalePWD is decision 0218's regression test: `xo lsp` started
// in the repository with a stale PWD (VS Code from the macOS Dock passes
// PWD=/) takes its working directory from the OS, so Go to Definition on
// http.Request in examples/notes-service/main_test.xo (line 70, character
// 50, zero based) answers std's http.xo, not null
// (gotchas/xo-working-dir-is-pwd.md).
func TestLSPStalePWD(t *testing.T) {
	if testing.Short() {
		t.Skip("runs xo lsp")
	}
	file := filepath.Join(repoRoot, "examples", "notes-service", "main_test.xo")
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(xoBin(t), "lsp")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "PWD=/")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errOut strings.Builder
	cmd.Stderr = &errOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		in.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}()
	out := bufio.NewReader(outPipe)
	send := func(id int, method string, params any) {
		msg := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
		if id > 0 {
			msg["id"] = id
		}
		body, _ := json.Marshal(msg)
		if _, err := fmt.Fprintf(in, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
			t.Fatal(err)
		}
	}
	// reply reads messages until the response with id (notifications
	// such as publishDiagnostics come in between).
	reply := func(id int) map[string]any {
		for {
			n := -1
			for {
				line, err := out.ReadString('\n')
				if err != nil {
					t.Fatalf("reading the server: %v\nstderr: %s", err, errOut.String())
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
					n, _ = strconv.Atoi(v)
				}
			}
			body := make([]byte, n)
			if _, err := io.ReadFull(out, body); err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatalf("%v: %s", err, body)
			}
			if got, ok := m["id"].(float64); ok && int(got) == id {
				return m
			}
		}
	}
	uri := "file://" + filepath.ToSlash(file)
	send(1, "initialize", map[string]any{"processId": nil, "rootUri": "file://" + filepath.ToSlash(repoRoot), "capabilities": map[string]any{}})
	reply(1)
	send(0, "initialized", map[string]any{})
	send(0, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "xo", "version": 1, "text": string(text)}})
	send(2, "textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": map[string]any{"line": 70, "character": 50}})
	def := reply(2)
	res, _ := json.Marshal(def["result"])
	if def["result"] == nil || !strings.Contains(string(res), "std/http.xo") {
		t.Errorf("definition of http.Request with PWD=/ = %s, want a location in std/http.xo\nstderr: %s", res, errOut.String())
	}
	send(3, "shutdown", nil)
	reply(3)
	send(0, "exit", nil)
}
