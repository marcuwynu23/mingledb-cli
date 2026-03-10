package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gomingleDB"
)

// captureStdoutStderr runs fn and returns combined stdout and stderr output.
func captureStdoutStderr(fn func()) string {
	origOut := os.Stdout
	origErr := os.Stderr
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	outDone := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outDone <- buf.String()
	}()

	fn()
	w.Close()
	return <-outDone
}

func TestSplitDotArgs(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{"empty", "", []string{}},
		{"single", ".exit", []string{".exit"}},
		{"with path", ".open /tmp/db", []string{".open", "/tmp/db"}},
		{"quoted", `.open "C:\path with spaces"`, []string{".open", "C:\\path with spaces"}},
		{"single quoted", ".open 'my db'", []string{".open", "my db"}},
		{"auth args", ".auth register alice secret", []string{".auth", "register", "alice", "secret"}},
		{"leading space", "  .help  ", []string{".help"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitDotArgs(tt.line)
			if len(got) != len(tt.want) {
				t.Errorf("splitDotArgs() len = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitDotArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSplitFirstWord(t *testing.T) {
	tests := []struct {
		line      string
		wantFirst string
		wantRest  string
	}{
		{"insert users {}", "insert", "users {}"},
		{"find users", "find", "users"},
		{"  findOne  coll {}", "findOne", "coll {}"},
		{"  ", "", ""},
		{"insert", "insert", ""},
	}
	for _, tt := range tests {
		first, rest := splitFirstWord(tt.line)
		if first != tt.wantFirst || rest != tt.wantRest {
			t.Errorf("splitFirstWord(%q) = %q, %q; want %q, %q", tt.line, first, rest, tt.wantFirst, tt.wantRest)
		}
	}
}

func TestSplitCollectionAndJSON(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		wantColl   string
		wantJSON   string
	}{
		{"simple", "users {}", "users", "{}"},
		{"with doc", "users {\"a\":1}", "users", "{\"a\":1}"},
		{"nested", "c { \"a\": { \"b\": 2 } }", "c", "{ \"a\": { \"b\": 2 } }"},
		{"no json", "users", "users", ""},
		{"empty", "", "", ""},
		{"leading space", "  col  {\"x\":1}  ", "col", "{\"x\":1}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coll, jsonStr := splitCollectionAndJSON(tt.s)
			if coll != tt.wantColl || jsonStr != tt.wantJSON {
				t.Errorf("splitCollectionAndJSON() = %q, %q; want %q, %q", coll, jsonStr, tt.wantColl, tt.wantJSON)
			}
		})
	}
}

func TestSplitCollectionQueryUpdate(t *testing.T) {
	tests := []struct {
		name        string
		s           string
		wantColl    string
		wantQuery   string
		wantUpdate  string
	}{
		{"both", "users {\"id\":1} {\"age\":31}", "users", "{\"id\":1}", "{\"age\":31}"},
		{"leading space", "  u {} {}", "u", "{}", "{}"},
		{"one brace", "u {}", "u", "{}", ""},
		{"nested", "c {\"a\":{\"b\":1}} {\"c\":2}", "c", "{\"a\":{\"b\":1}}", "{\"c\":2}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coll, q, u := splitCollectionQueryUpdate(tt.s)
			if coll != tt.wantColl || q != tt.wantQuery || u != tt.wantUpdate {
				t.Errorf("splitCollectionQueryUpdate() = %q, %q, %q; want %q, %q, %q", coll, q, u, tt.wantColl, tt.wantQuery, tt.wantUpdate)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	// valid
	m, err := parseJSON("{\"a\":1,\"b\":\"x\"}")
	if err != nil {
		t.Fatal("parseJSON valid:", err)
	}
	if m["a"].(float64) != 1 || m["b"].(string) != "x" {
		t.Errorf("parseJSON = %v", m)
	}
	// invalid
	_, err = parseJSON("{invalid}")
	if err == nil {
		t.Error("parseJSON invalid: expected error")
	}
	// empty object
	m, err = parseJSON("{}")
	if err != nil || len(m) != 0 {
		t.Errorf("parseJSON {}: err=%v len=%d", err, len(m))
	}
}

func TestRunCommand_Exit(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	sess := &session{db: gomingleDB.New(absDir), dbDir: absDir}
	out := captureStdoutStderr(func() {
		runCommand(sess, ".exit")
	})
	if !strings.Contains(out, "Bye.") {
		t.Errorf("expected Bye., got %q", out)
	}
}

func TestRunDotCommand_Help(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	sess := &session{db: gomingleDB.New(absDir), dbDir: absDir}
	out := captureStdoutStderr(func() {
		runDotCommand(sess, ".help")
	})
	if !strings.Contains(out, ".exit") || !strings.Contains(out, "insert") {
		t.Errorf("help should mention .exit and insert, got %q", out)
	}
}

func TestRunDotCommand_Databases(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	sess := &session{db: gomingleDB.New(absDir), dbDir: absDir}
	out := captureStdoutStderr(func() {
		runDotCommand(sess, ".databases")
	})
	if !strings.Contains(out, absDir) {
		t.Errorf(".databases should print db dir, got %q", out)
	}
}

func TestRunDotCommand_Open(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	sess := &session{db: gomingleDB.New(absDir), dbDir: absDir}
	newDir := t.TempDir()
	absNew, _ := filepath.Abs(newDir)
	out := captureStdoutStderr(func() {
		runDotCommand(sess, ".open "+absNew)
	})
	if !strings.Contains(out, "Opened") || !strings.Contains(out, absNew) {
		t.Errorf(".open should open and print path, got %q", out)
	}
	if sess.dbDir != absNew {
		t.Errorf("sess.dbDir = %q, want %q", sess.dbDir, absNew)
	}
}

func TestRunDotCommand_Collections(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	sess := &session{db: gomingleDB.New(absDir), dbDir: absDir}
	// ensure at least one collection exists
	_ = sess.db.InsertOne("testcoll", map[string]interface{}{"a": 1.0})
	out := captureStdoutStderr(func() {
		runDotCommand(sess, ".collections")
	})
	if !strings.Contains(out, "testcoll") {
		t.Errorf(".collections should list testcoll, got %q", out)
	}
}

func TestRunDataCommand_InsertFind(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	db := gomingleDB.New(absDir)

	// insert
	out := captureStdoutStderr(func() {
		runDataCommand(db, "insert users {\"name\":\"Alice\",\"age\":30}")
	})
	if !strings.Contains(out, "Inserted") {
		t.Errorf("insert should confirm, got %q", out)
	}

	// find
	out = captureStdoutStderr(func() {
		runDataCommand(db, "find users {}")
	})
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "30") {
		t.Errorf("find should return doc, got %q", out)
	}
	if !strings.Contains(out, "1 document") {
		t.Errorf("find should report count, got %q", out)
	}
}

func TestRunDataCommand_FindOne(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	db := gomingleDB.New(absDir)
	_ = db.InsertOne("u", map[string]interface{}{"email": "a@b.com", "n": 1.0})

	out := captureStdoutStderr(func() {
		runDataCommand(db, "findOne u {\"email\":\"a@b.com\"}")
	})
	if !strings.Contains(out, "a@b.com") {
		t.Errorf("findOne should return doc, got %q", out)
	}
}

func TestRunDataCommand_Update(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	db := gomingleDB.New(absDir)
	_ = db.InsertOne("u", map[string]interface{}{"id": 1.0, "age": 25.0})

	out := captureStdoutStderr(func() {
		runDataCommand(db, "update u {\"id\":1} {\"age\":31}")
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("update should confirm, got %q", out)
	}

	// verify
	docs, _ := db.Find("u", map[string]interface{}{})
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if age, ok := docs[0]["age"].(float64); !ok || age != 31 {
		t.Errorf("age should be 31, got %v", docs[0]["age"])
	}
}

func TestRunDataCommand_Delete(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	db := gomingleDB.New(absDir)
	_ = db.InsertOne("u", map[string]interface{}{"email": "x@y.com"})

	out := captureStdoutStderr(func() {
		runDataCommand(db, "delete u {\"email\":\"x@y.com\"}")
	})
	if !strings.Contains(out, "Deleted") {
		t.Errorf("delete should confirm, got %q", out)
	}

	docs, _ := db.Find("u", map[string]interface{}{})
	if len(docs) != 0 {
		t.Errorf("expected 0 docs after delete, got %d", len(docs))
	}
}

func TestRunDataCommand_Schema(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	db := gomingleDB.New(absDir)

	out := captureStdoutStderr(func() {
		runDataCommand(db, "schema users {\"name\":{\"type\":\"string\",\"required\":true},\"age\":{\"type\":\"number\"}}")
	})
	if !strings.Contains(out, "Schema defined") || !strings.Contains(out, "users") {
		t.Errorf("schema should confirm, got %q", out)
	}

	schema, ok := db.GetSchema("users")
	if !ok || schema["name"].Type != "string" || !schema["name"].Required {
		t.Errorf("GetSchema users: ok=%v schema=%v", ok, schema)
	}
}

func TestRunDataCommand_UnknownCommand(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	db := gomingleDB.New(absDir)
	out := captureStdoutStderr(func() {
		runDataCommand(db, "unknown x y")
	})
	if !strings.Contains(out, "Unknown command") {
		t.Errorf("unknown command should error, got %q", out)
	}
}

func TestRunDataCommand_InsertBadJSON(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	db := gomingleDB.New(absDir)
	out := captureStdoutStderr(func() {
		runDataCommand(db, "insert users {invalid}")
	})
	if !strings.Contains(out, "json") {
		t.Errorf("insert bad json should error, got %q", out)
	}
}

func TestRunDotCommand_Unknown(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	sess := &session{db: gomingleDB.New(absDir), dbDir: absDir}
	out := captureStdoutStderr(func() {
		runDotCommand(sess, ".unknown")
	})
	if !strings.Contains(out, "Unknown command") {
		t.Errorf("unknown dot command should error, got %q", out)
	}
}

func TestRunCommand_RoutesDotVsData(t *testing.T) {
	dir := t.TempDir()
	absDir, _ := filepath.Abs(dir)
	sess := &session{db: gomingleDB.New(absDir), dbDir: absDir}

	// .help is dot
	out := captureStdoutStderr(func() {
		runCommand(sess, ".help")
	})
	if !strings.Contains(out, ".exit") {
		t.Errorf("runCommand .help should run dot, got %q", out)
	}

	// insert is data
	_ = sess.db.InsertOne("x", map[string]interface{}{"a": 1.0})
	out = captureStdoutStderr(func() {
		runCommand(sess, "find x {}")
	})
	if !strings.Contains(out, "document") {
		t.Errorf("runCommand find should run data, got %q", out)
	}
}
