// mingledb-cli is an interactive SQLite-style shell for mingleDB (.mgdb) databases.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/mingledb/gomingleDB"
	"github.com/reeflective/readline"
)

var version = "1.0" // set by -ldflags on release build

const (
	prompt            = "mingledb> "
	contPrompt        = "...> "
	accentColor       = "\x1b[38;2;21;153;144m" // #159990
	whiteColor        = "\x1b[97m"
	memoryColor       = "\x1b[38;2;230;74;25m" // #e64a19
	resetColor        = "\x1b[0m"
	bold              = "\x1b[1m"
	historyFile       = ".mgdb_history"
	styledMemoryLabel = "(" + bold + memoryColor + "memory" + resetColor + ")"
)

type session struct {
	db          *gomingleDB.MingleDB
	dbDir       string
	displayPath string
	tempDir     string // when set, DB is in-memory (temp dir); cleaned up on exit or .output/.open
	outputMode  string // json, table, csv, line, list
}

// resolveDBPaths returns database path input and display path.
func resolveDBPaths(path string) (dbPath, displayPath string) {
	abs, _ := filepath.Abs(path)
	return abs, abs
}

// Backward-compatible helper used by tests.
func resolveDBPath(path string) string {
	dbPath, _ := resolveDBPaths(path)
	return dbPath
}

func main() {
	var sess *session
	if len(os.Args) > 1 {
		dbDir, displayPath := resolveDBPaths(os.Args[1])
		sess = &session{db: gomingleDB.New(dbDir), dbDir: dbDir, displayPath: displayPath, outputMode: "json"}
	} else {
		tempDir, err := os.MkdirTemp("", "mgdb-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "mgdb: could not create in-memory DB:", err)
			os.Exit(1)
		}
		sess = &session{db: gomingleDB.New(tempDir), dbDir: tempDir, displayPath: "(memory)", tempDir: tempDir, outputMode: "json"}
	}
	defer func() {
		if sess != nil && sess.tempDir != "" {
			os.RemoveAll(sess.tempDir)
		}
	}()

	fmt.Fprintf(os.Stderr, "%smgdb %s%s\nType .help for commands.\n\n", accentColor, version, resetColor)

	rl := readline.NewShell()
    rl.Prompt.Primary(func() string { return accentColor + prompt + resetColor })
    rl.Prompt.Secondary(func() string { return accentColor + contPrompt + resetColor })
    _ = rl.Config.Set("history-autosuggest", true)
	// Disable features that cause display distortion
	_ = rl.Config.Set("history-autosuggest", false)
	_ = rl.Config.Set("syntax-highlighting", false)
	_ = rl.Config.Set("completions-on-trigger", false)
	_ = rl.Config.Set("completion-menu", false)
	rl.AcceptMultiline = func(line []rune) bool {
		return isInputComplete(string(line))
	}
	rl.Completer = buildCompleter(sess)
	if historyPath, ok := resolveHistoryPath(); ok {
		seedHistoryFile(historyPath)
		rl.History.AddFromFile("mgdb", historyPath)
	}

	for {
		line, err := rl.Readline()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if errors.Is(err, readline.ErrInterrupt) {
				fmt.Fprintln(os.Stderr)
				continue
			}
			fmt.Fprintln(os.Stderr, "read error:", err)
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			fmt.Println()
			continue
		}

		done := runCommand(sess, line)
		if done {
			break
		}
	}
}

func runCommand(sess *session, line string) (exit bool) {
	// Dot commands
	if strings.HasPrefix(line, ".") {
		return runDotCommand(sess, line)
	}
	// Data commands
	if sess.db == nil {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "No database open. Use .open PATH")
		return false
	}
	return runDataCommand(sess.db, sess, line)
}

func runDotCommand(sess *session, line string) (exit bool) {
	parts := splitDotArgs(line)
	cmd := strings.ToLower(parts[0])
	db := sess.db
	switch cmd {
	case ".exit", ".quit":
		fmt.Fprintln(os.Stderr, "Bye.")
		return true
	case ".help", ".h":
		fmt.Fprint(os.Stderr, buildHelpMessage())
		return false
	case ".databases":
		if db == nil {
			fmt.Println("(no database open)")
			return false
		}
		if sess.tempDir != "" {
			fmt.Println(styledMemoryLabel)
		} else {
			fmt.Println(sess.displayPath)
		}
		return false
	case ".open":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: .open PATH  (PATH can be a directory or a .mgdb file)")
			return false
		}
		if sess.tempDir != "" {
			os.RemoveAll(sess.tempDir)
			sess.tempDir = ""
		}
		path := parts[1]
		dbDir, displayPath := resolveDBPaths(path)
		sess.dbDir = dbDir
		sess.displayPath = displayPath
		sess.db = gomingleDB.New(dbDir)
		fmt.Fprintln(os.Stderr, "Opened", displayPath)
		return false
	case ".output":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: .output PATH  (directory or .mgdb path to save in-memory database)")
			return false
		}
		if sess.tempDir == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Database is already on disk. Use .open to switch, or .output only applies to in-memory DB.")
			return false
		}
		path := parts[1]
		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "output path:", err)
			return false
		}
		outDB := gomingleDB.New(absPath)
		if err := os.MkdirAll(filepath.Dir(outDB.DBDir()), 0755); err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "output:", err)
			return false
		}
		colls, err := db.ListCollections()
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "output:", err)
			return false
		}
		for _, col := range colls {
			if schema, ok := db.GetSchema(col); ok {
				outDB.DefineSchema(col, schema)
			}
			docs, err := db.Find(col, map[string]interface{}{})
			if err != nil {
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, "output:", col, err)
				return false
			}
			for _, doc := range docs {
				if err := outDB.InsertOne(col, doc); err != nil {
					fmt.Fprintln(os.Stderr)
					fmt.Fprintln(os.Stderr, "output:", err)
					return false
				}
			}
		}
		os.RemoveAll(sess.tempDir)
		sess.tempDir = ""
		sess.dbDir = outDB.DBDir()
		sess.displayPath = absPath
		sess.db = outDB
		fmt.Fprintln(os.Stderr, "Saved to", absPath)
		return false
	case ".collections":
		if db == nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "No database open. Use .open PATH")
			return false
		}
		colls, err := db.ListCollections()
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "collections:", err)
			return false
		}
		sort.Strings(colls)
		for _, c := range colls {
			fmt.Println(c)
		}
		return false
	case ".schema":
		if db == nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "No database open. Use .open PATH")
			return false
		}
		if len(parts) < 2 {
			// List collections that have schema
			colls, _ := db.ListCollections()
			sort.Strings(colls)
			for _, c := range colls {
				if _, ok := db.GetSchema(c); ok {
					fmt.Println(c)
				}
			}
			return false
		}
		name := parts[1]
		schema, ok := db.GetSchema(name)
		if !ok {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "No schema for collection:", name)
			return false
		}
		b, _ := json.MarshalIndent(schema, "  ", "  ")
		fmt.Println(string(b))
		return false
	case ".auth":
		if db == nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "No database open. Use .open PATH")
			return false
		}
		return runAuth(db, parts)
	case ".system", ".sys":
		cmdLine := strings.TrimSpace(strings.TrimPrefix(line, parts[0]))
		if cmdLine == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: .system/.sys CMD [args...]")
			return false
		}
		return runSystemCommand(cmdLine)
	case ".mode":
		return runMode(sess, parts)
	case ".clear":
		fmt.Print("\033[H\033[2J")
		return false
	default:
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Unknown command: %s (use .help)\n", cmd)
		return false
	}
}

func runSystemCommand(cmdLine string) bool {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", cmdLine)
	} else {
		cmd = exec.Command("sh", "-c", cmdLine)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Stderr.Write(exitErr.Stderr)
		}
		fmt.Fprintln(os.Stderr, "system:", err)
		return false
	}
	return false
}

func splitDotArgs(line string) []string {
	line = strings.TrimSpace(line)
	var parts []string
	for line != "" {
		line = strings.TrimLeft(line, " \t")
		if line == "" {
			break
		}
		if line[0] == '"' || line[0] == '\'' {
			quote := line[0]
			end := strings.IndexByte(line[1:], quote)
			if end == -1 {
				parts = append(parts, line)
				break
			}
			end++
			parts = append(parts, line[1:end])
			line = line[end+1:]
			continue
		}
		i := 0
		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		parts = append(parts, line[:i])
		line = line[i:]
	}
	return parts
}

var authUser string

func runAuth(db *gomingleDB.MingleDB, parts []string) bool {
	if len(parts) < 2 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Usage: .auth register|login|logout|status [args]")
		return false
	}
	sub := strings.ToLower(parts[1])
	switch sub {
	case "register":
		if len(parts) < 4 {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: .auth register USERNAME PASSWORD")
			return false
		}
		if err := db.RegisterUser(parts[2], parts[3]); err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "register:", err)
			return false
		}
		fmt.Fprintln(os.Stderr, "User registered.")
	case "login":
		if len(parts) < 4 {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: .auth login USERNAME PASSWORD")
			return false
		}
		if err := db.Login(parts[2], parts[3]); err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "login:", err)
			return false
		}
		authUser = parts[2]
		fmt.Fprintln(os.Stderr, "Logged in as", authUser)
	case "logout":
		if authUser != "" {
			db.Logout(authUser)
			authUser = ""
		}
		fmt.Fprintln(os.Stderr, "Logged out.")
	case "status":
		if authUser != "" && db.IsAuthenticated(authUser) {
			fmt.Println(authUser)
		} else {
			fmt.Println("(not logged in)")
		}
	default:
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Usage: .auth register|login|logout|status [args]")
	}
	return false
}

var validModes = []string{"json", "table", "csv", "line", "list"}

func runMode(sess *session, parts []string) bool {
	if len(parts) < 2 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Current mode: %s\n", sess.outputMode)
		fmt.Fprintf(os.Stderr, "Valid modes: %s\n", strings.Join(validModes, ", "))
		return false
	}
	mode := strings.ToLower(parts[1])
	for _, m := range validModes {
		if m == mode {
			sess.outputMode = mode
			fmt.Fprintln(os.Stderr, "Output mode:", mode)
			return false
		}
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Invalid mode: %s. Valid modes: %s\n", mode, strings.Join(validModes, ", "))
	return false
}

func runDataCommand(db *gomingleDB.MingleDB, sess *session, line string) (exit bool) {
	cmd, rest := splitFirstWord(line)
	cmd = strings.ToLower(cmd)
	if rest == "" {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Missing arguments. Use .help")
		return false
	}

	switch cmd {
	case "insert":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: insert COLLECTION {json}")
			return false
		}
		doc, err := parseJSON(jsonStr)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "insert json:", err)
			return false
		}
		if err := db.InsertOne(col, doc); err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "insert:", err)
			return false
		}
		fmt.Fprintln(os.Stderr, "Inserted 1 document.")
	case "find":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: find COLLECTION [filter json]")
			return false
		}
		filter := make(map[string]interface{})
		if jsonStr != "" {
			var err error
			filter, err = parseJSONObject(jsonStr)
			if err != nil {
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, "find filter:", err)
				return false
			}
		}
		docs, err := db.Find(col, filter)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "find:", err)
			return false
		}
		printDocs(docs, sess.outputMode)
	case "findone":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: findOne COLLECTION [filter json]")
			return false
		}
		filter := make(map[string]interface{})
		if jsonStr != "" {
			var err error
			filter, err = parseJSONObject(jsonStr)
			if err != nil {
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, "findOne filter:", err)
				return false
			}
		}
		doc, err := db.FindOne(col, filter)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "findOne:", err)
			return false
		}
		if doc == nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "(no document)")
			return false
		}
		printDocs([]map[string]interface{}{doc}, sess.outputMode)
	case "update":
		col, queryStr, updateStr := splitCollectionQueryUpdate(rest)
		if col == "" || queryStr == "" || updateStr == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: update COLLECTION query_json update_json")
			return false
		}
		query, err := parseJSONObject(queryStr)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "update query:", err)
			return false
		}
		update, err := parseJSONObject(updateStr)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "update update:", err)
			return false
		}
		ok, err := db.UpdateOne(col, query, update)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "update:", err)
			return false
		}
		if ok {
			fmt.Fprintln(os.Stderr, "Updated 1 document.")
		} else {
			fmt.Fprintln(os.Stderr, "No document matched.")
		}
	case "delete":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: delete COLLECTION query_json")
			return false
		}
		query, err := parseJSONObject(jsonStr)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "delete query:", err)
			return false
		}
		ok, err := db.DeleteOne(col, query)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "delete:", err)
			return false
		}
		if ok {
			fmt.Fprintln(os.Stderr, "Deleted 1 document.")
		} else {
			fmt.Fprintln(os.Stderr, "No document matched.")
		}
	case "schema":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" || jsonStr == "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Usage: schema COLLECTION schema_json")
			return false
		}
		raw, err := parseJSON(jsonStr)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "schema json:", err)
			return false
		}
		schema := make(gomingleDB.SchemaDefinition)
		for k, v := range raw {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			rule := gomingleDB.SchemaRule{}
			if t, _ := vm["type"].(string); t != "" {
				rule.Type = t
			}
			if r, _ := vm["required"].(bool); r {
				rule.Required = true
			}
			if u, _ := vm["unique"].(bool); u {
				rule.Unique = true
			}
			schema[k] = rule
		}
		db.DefineSchema(col, schema)
		fmt.Fprintln(os.Stderr, "Schema defined for", col)
	default:
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Unknown command: %s (use .help)\n", cmd)
	}
	return false
}

func splitFirstWord(s string) (first, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	if i == 0 {
		return "", s
	}
	return s[:i], strings.TrimSpace(s[i:])
}

// splitCollectionAndJSON returns collection name and the JSON part (from first { to end).
func splitCollectionAndJSON(s string) (collection, jsonStr string) {
	s = strings.TrimLeft(s, " \t")
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '{' {
		i++
	}
	collection = strings.TrimSpace(s[:i])
	rest := strings.TrimLeft(s[i:], " \t")
	start := strings.Index(rest, "{")
	if start == -1 {
		return collection, ""
	}
	// Find matching closing brace
	depth := 0
	for j := start; j < len(rest); j++ {
		if rest[j] == '{' {
			depth++
		} else if rest[j] == '}' {
			depth--
			if depth == 0 {
				return collection, rest[start : j+1]
			}
		}
	}
	return collection, rest[start:]
}

// splitCollectionQueryUpdate splits "col {...} {...}" into col, queryJson, updateJson.
func splitCollectionQueryUpdate(s string) (collection, queryStr, updateStr string) {
	s = strings.TrimLeft(s, " \t")
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '{' {
		i++
	}
	collection = strings.TrimSpace(s[:i])
	rest := strings.TrimLeft(s[i:], " \t")
	// First {...}
	start := strings.Index(rest, "{")
	if start == -1 {
		return collection, "", ""
	}
	depth := 0
	for j := start; j < len(rest); j++ {
		if rest[j] == '{' {
			depth++
		} else if rest[j] == '}' {
			depth--
			if depth == 0 {
				queryStr = rest[start : j+1]
				rest = strings.TrimLeft(rest[j+1:], " \t")
				break
			}
		}
	}
	start = strings.Index(rest, "{")
	if start == -1 {
		return collection, queryStr, ""
	}
	depth = 0
	for j := start; j < len(rest); j++ {
		if rest[j] == '{' {
			depth++
		} else if rest[j] == '}' {
			depth--
			if depth == 0 {
				updateStr = rest[start : j+1]
				return collection, queryStr, updateStr
			}
		}
	}
	updateStr = rest[start:]
	return collection, queryStr, updateStr
}

func parseJSON(s string) (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func parseJSONObject(s string) (map[string]interface{}, error) {
	return parseJSON(s)
}

func isInputComplete(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if strings.HasSuffix(line, "\\") {
		return false
	}
	if strings.HasPrefix(line, ".") {
		return true
	}
	cmd, _ := splitFirstWord(line)
	if cmd == "" || !strings.Contains(line, "{") {
		return true
	}
	return hasRequiredJSONObjects(line, expectedJSONObjectCount(strings.ToLower(cmd)))
}

func hasBalancedJSONBraces(s string) bool {
	inString := false
	escaped := false
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			if depth > 0 {
				depth--
			}
		}
	}
	return depth == 0
}

func expectedJSONObjectCount(cmd string) int {
	switch cmd {
	case "update":
		return 2
	case "insert", "find", "findone", "delete", "schema":
		return 1
	default:
		return 1
	}
}

func hasRequiredJSONObjects(s string, required int) bool {
	if required <= 0 {
		return true
	}
	inString := false
	escaped := false
	depth := 0
	completed := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
			continue
		}
		if ch == '}' {
			if depth > 0 {
				depth--
				if depth == 0 {
					completed++
				}
			}
		}
	}
	if completed < required {
		return false
	}
	return depth == 0 && !inString
}

func buildCompleter(sess *session) func(line []rune, cursor int) readline.Completions {
	dotCommands := []string{
		".help", ".exit", ".quit", ".databases", ".open", ".collections",
		".schema", ".auth", ".system", ".sys", ".output", ".mode", ".clear",
	}
	dataCommands := []string{"insert", "find", "findOne", "update", "delete", "schema"}
	authCommands := []string{"register", "login", "logout", "status"}

	return func(line []rune, cursor int) readline.Completions {
		if cursor < 0 || cursor > len(line) {
			cursor = len(line)
		}
		input := string(line[:cursor])
		trimmed := strings.TrimLeft(input, " \t")
		if trimmed == "" {
			return readline.CompleteValues(append(dotCommands, dataCommands...)...).Tag("commands")
		}

		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			return readline.CompleteValues(append(dotCommands, dataCommands...)...).Tag("commands")
		}

		first := strings.ToLower(fields[0])
		endsWithSpace := strings.HasSuffix(input, " ")

		if strings.HasPrefix(first, ".") {
			if len(fields) == 1 && !endsWithSpace {
				return readline.CompleteValues(dotCommands...).Tag("dot commands")
			}
			if first == ".auth" && (len(fields) == 1 || (len(fields) == 2 && !endsWithSpace)) {
				return readline.CompleteValues(authCommands...).Tag("auth")
			}
			return readline.Message("dot command")
		}

		if len(fields) == 1 && !endsWithSpace {
			return readline.CompleteValues(dataCommands...).Tag("data commands")
		}

		if sess != nil && sess.db != nil && (len(fields) == 2 && !endsWithSpace || len(fields) >= 2 && endsWithSpace) {
			colls, err := sess.db.ListCollections()
			if err == nil && len(colls) > 0 {
				sort.Strings(colls)
				return readline.CompleteValues(colls...).Tag("collections")
			}
		}

		return readline.Message("json")
	}
}

func resolveHistoryPath() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, historyFile), true
}

func seedHistoryFile(path string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	seed := strings.Join([]string{
		".help",
		".databases",
		".collections",
		".schema",
		".system",
		".sys",
		"find",
		"insert",
		"update",
		"delete",
		"schema",
	}, "\n") + "\n"
	_ = os.WriteFile(path, []byte(seed), 0644)
}

// resolveRegexInFilter converts $regex string in filter to *regexp.Regexp for gomingleDB.
func resolveRegexInFilter(m map[string]interface{}) {
	for _, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			if pattern, ok := vm["$regex"].(string); ok {
				opts, _ := vm["$options"].(string)
				if opts == "i" {
					pattern = "(?i)" + pattern
				}
				if re, err := regexp.Compile(pattern); err == nil {
					vm["$regex"] = re
				}
			}
			resolveRegexInFilter(vm)
		}
	}
}

func printDocs(docs []map[string]interface{}, mode string) {
	switch mode {
	case "table":
		printDocsTable(docs)
	case "csv":
		printDocsCSV(docs)
	case "line":
		printDocsLine(docs)
	case "list":
		printDocsList(docs)
	default: // json
		printDocsJSON(docs)
	}
	// Ensure terminal scrolls properly
	fmt.Println()
}

func printDocsJSON(docs []map[string]interface{}) {
	if len(docs) == 0 {
		return
	}
	// Output as JSON array
	b, _ := json.MarshalIndent(docs, "", "  ")
	fmt.Println(string(b))
	fmt.Fprintf(os.Stderr, "(%d document(s))\n", len(docs))
}

func printDocsTable(docs []map[string]interface{}) {
	if len(docs) == 0 {
		return
	}
	// Get all unique keys
	keys := make(map[string]bool)
	for _, doc := range docs {
		for k := range doc {
			keys[k] = true
		}
	}
	var cols []string
	for k := range keys {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	// Calculate column widths
	widths := make(map[string]int)
	for _, k := range cols {
		widths[k] = len(k)
	}
	for _, doc := range docs {
		for _, k := range cols {
			v := ""
			if doc[k] != nil {
				v = fmt.Sprintf("%v", doc[k])
			}
			if len(v) > widths[k] {
				widths[k] = len(v)
			}
		}
	}

	// Print header
	for _, k := range cols {
		fmt.Printf("%-*s  ", widths[k], k)
	}
	fmt.Println()
	for _, k := range cols {
		fmt.Print(strings.Repeat("-", widths[k]) + "  ")
	}
	fmt.Println()

	// Print rows
	for _, doc := range docs {
		for _, k := range cols {
			v := ""
			if doc[k] != nil {
				v = fmt.Sprintf("%v", doc[k])
			}
			fmt.Printf("%-*s  ", widths[k], v)
		}
		fmt.Println()
	}
	fmt.Fprintf(os.Stderr, "(%d document(s))\n", len(docs))
}

func printDocsCSV(docs []map[string]interface{}) {
	if len(docs) == 0 {
		return
	}
	// Get all unique keys
	keys := make(map[string]bool)
	for _, doc := range docs {
		for k := range doc {
			keys[k] = true
		}
	}
	var cols []string
	for k := range keys {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	// Print header
	fmt.Println(strings.Join(cols, ","))

	// Print rows
	for _, doc := range docs {
		vals := make([]string, len(cols))
		for i, k := range cols {
			v := ""
			if doc[k] != nil {
				v = fmt.Sprintf("%v", doc[k])
			}
			vals[i] = v
		}
		fmt.Println(strings.Join(vals, ","))
	}
}

func printDocsLine(docs []map[string]interface{}) {
	for _, doc := range docs {
		for k, v := range doc {
			fmt.Printf("%s = %v\n", k, v)
		}
		fmt.Println()
	}
}

func printDocsList(docs []map[string]interface{}) {
	for i, doc := range docs {
		fmt.Printf("Document %d:\n", i+1)
		for k, v := range doc {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}
}

func buildHelpMessage() string {
	return fmt.Sprintf(`%s%sMingleDB CLI Help%s

%sDot Commands%s
%s  .help              Show this help
  .exit, .quit       Exit the CLI
  .databases         Show current database path
  .open PATH         Open database (PATH: directory or .mgdb file)
  .collections       List collection names
  .schema [NAME]     Show schema for collection (or list if no NAME)
  .auth register U P Register user U with password P
  .auth login U P    Log in as user U
  .auth logout       Log out current session
  .auth status       Show logged-in user
  .system/.sys CMD   Run system command (e.g. .system ls -la)
  .output PATH       Save in-%smemory%s database to file
  .mode [MODE]       Set output mode: json, table, csv, line, list
  .clear             Clear the terminal screen

%sData Commands%s %s(JSON docs / filters)%s
%s  insert COLL DOC        e.g. insert users {"name":"Alice","age":30}
  find COLL [FILTER]     e.g. find users {"age":{"$gte":18}}
  findOne COLL [FILTER]  e.g. findOne users {"email":"a@b.com"}
  update COLL QUERY UPDATE e.g. update users {"id":1} {"age":31}
  delete COLL QUERY      e.g. delete users {"email":"x@y.com"}
  schema COLL DEF        e.g. schema users {"name":{"type":"string","required":true}}%s

%sTip:%s %suse .databases to confirm whether you're on-disk or %smemory%s%s.
`,
		bold, accentColor, resetColor,
		bold+accentColor, resetColor,
		whiteColor,
		bold+memoryColor, resetColor,
		bold+accentColor, resetColor, whiteColor, resetColor,
		whiteColor, resetColor,
		bold+accentColor, resetColor, whiteColor, bold+memoryColor, resetColor, whiteColor,
	)
}
