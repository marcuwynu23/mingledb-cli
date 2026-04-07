// mingledb-cli is an interactive SQLite-style shell for mingleDB (.mgdb) databases.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/mingledb/gomingleDB"
)

var version = "1.0" // set by -ldflags on release build

const (
	prompt     = "mingledb> "
	helpMessage = `Dot commands:
  .exit, .quit       Exit the CLI
  .help              Show this help
  .databases         Show current database directory
  .open PATH         Open database (PATH: directory or .mgdb file)
  .collections       List collection names
  .schema [NAME]     Show schema for collection (or list if no NAME)
  .auth register U P Register user U with password P
  .auth login U P    Log in as user U
  .auth logout       Log out current session
  .auth status       Show logged-in user
  .system CMD        Run system command (e.g. .system ls -la)
  .output PATH       Save in-memory database to file (directory or .mgdb path)

Data commands (JSON for docs/filters):
  insert COLL DOC                    e.g. insert users {"name":"Alice","age":30}
  find COLL [FILTER]                 e.g. find users {"age":{"$gte":18}}
  findOne COLL [FILTER]              e.g. findOne users {"email":"a@b.com"}
  update COLL QUERY UPDATE            e.g. update users {"id":1} {"age":31}
  delete COLL QUERY                  e.g. delete users {"email":"x@y.com"}
  schema COLL DEF                    e.g. schema users {"name":{"type":"string","required":true}}
`
)

type session struct {
	db      *gomingleDB.MingleDB
	dbDir   string
	tempDir string // when set, DB is in-memory (temp dir); cleaned up on exit or .output/.open
}

// resolveDBPath returns the database directory. If path ends with .mgdb, use its directory (so a file path means "use the dir containing this file").
func resolveDBPath(path string) string {
	abs, _ := filepath.Abs(path)
	if strings.HasSuffix(strings.ToLower(abs), ".mgdb") {
		return filepath.Dir(abs)
	}
	return abs
}

func main() {
	var sess *session
	if len(os.Args) > 1 {
		dbDir := resolveDBPath(os.Args[1])
		sess = &session{db: gomingleDB.New(dbDir), dbDir: dbDir}
	} else {
		tempDir, err := os.MkdirTemp("", "mgdb-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "mgdb: could not create in-memory DB:", err)
			os.Exit(1)
		}
		sess = &session{db: gomingleDB.New(tempDir), dbDir: tempDir, tempDir: tempDir}
	}
	defer func() {
		if sess != nil && sess.tempDir != "" {
			os.RemoveAll(sess.tempDir)
		}
	}()

	fmt.Fprintf(os.Stderr, "mgdb %s\nType .help for commands.\n\n", version)

	scanner := bufio.NewScanner(os.Stdin)
	var lineBuf strings.Builder
	for {
		fmt.Fprint(os.Stderr, prompt)
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Multi-line: if line ends with \, continue reading
		for strings.HasSuffix(line, "\\") {
			line = strings.TrimSuffix(line, "\\")
			lineBuf.Reset()
			lineBuf.WriteString(line)
			fmt.Fprint(os.Stderr, "   ...> ")
			if !scanner.Scan() {
				break
			}
			line = lineBuf.String() + " " + strings.TrimSpace(scanner.Text())
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		done := runCommand(sess, line)
		if done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
	}
}

func runCommand(sess *session, line string) (exit bool) {
	// Dot commands
	if strings.HasPrefix(line, ".") {
		return runDotCommand(sess, line)
	}
	// Data commands
	if sess.db == nil {
		fmt.Fprintln(os.Stderr, "No database open. Use .open PATH")
		return false
	}
	return runDataCommand(sess.db, line)
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
		fmt.Fprint(os.Stderr, helpMessage)
		return false
	case ".databases":
		if db == nil {
			fmt.Println("(no database open)")
			return false
		}
		if sess.tempDir != "" {
			fmt.Println("(memory)")
		} else {
			fmt.Println(db.DBDir())
		}
		return false
	case ".open":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: .open PATH  (PATH can be a directory or a .mgdb file)")
			return false
		}
		if sess.tempDir != "" {
			os.RemoveAll(sess.tempDir)
			sess.tempDir = ""
		}
		path := parts[1]
		absPath := resolveDBPath(path)
		sess.dbDir = absPath
		sess.db = gomingleDB.New(absPath)
		fmt.Fprintln(os.Stderr, "Opened", absPath)
		return false
	case ".output":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: .output PATH  (directory or .mgdb path to save in-memory database)")
			return false
		}
		if sess.tempDir == "" {
			fmt.Fprintln(os.Stderr, "Database is already on disk. Use .open to switch, or .output only applies to in-memory DB.")
			return false
		}
		path := parts[1]
		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "output path:", err)
			return false
		}
		if strings.HasSuffix(strings.ToLower(absPath), ".mgdb") {
			absPath = filepath.Dir(absPath)
		}
		if err := os.MkdirAll(absPath, 0755); err != nil {
			fmt.Fprintln(os.Stderr, "output:", err)
			return false
		}
		outDB := gomingleDB.New(absPath)
		colls, err := db.ListCollections()
		if err != nil {
			fmt.Fprintln(os.Stderr, "output:", err)
			return false
		}
		for _, col := range colls {
			if schema, ok := db.GetSchema(col); ok {
				outDB.DefineSchema(col, schema)
			}
			docs, err := db.Find(col, map[string]interface{}{})
			if err != nil {
				fmt.Fprintln(os.Stderr, "output:", col, err)
				return false
			}
			for _, doc := range docs {
				if err := outDB.InsertOne(col, doc); err != nil {
					fmt.Fprintln(os.Stderr, "output:", err)
					return false
				}
			}
		}
		os.RemoveAll(sess.tempDir)
		sess.tempDir = ""
		sess.dbDir = absPath
		sess.db = outDB
		fmt.Fprintln(os.Stderr, "Saved to", absPath)
		return false
	case ".collections":
		if db == nil {
			fmt.Fprintln(os.Stderr, "No database open. Use .open PATH")
			return false
		}
		colls, err := db.ListCollections()
		if err != nil {
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
			fmt.Fprintln(os.Stderr, "No schema for collection:", name)
			return false
		}
		b, _ := json.MarshalIndent(schema, "  ", "  ")
		fmt.Println(string(b))
		return false
	case ".auth":
		if db == nil {
			fmt.Fprintln(os.Stderr, "No database open. Use .open PATH")
			return false
		}
		return runAuth(db, parts)
	case ".system":
		cmdLine := strings.TrimSpace(strings.TrimPrefix(line, ".system"))
		if cmdLine == "" {
			fmt.Fprintln(os.Stderr, "Usage: .system CMD [args...]")
			return false
		}
		return runSystemCommand(cmdLine)
	default:
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
		fmt.Fprintln(os.Stderr, "Usage: .auth register|login|logout|status [args]")
		return false
	}
	sub := strings.ToLower(parts[1])
	switch sub {
	case "register":
		if len(parts) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: .auth register USERNAME PASSWORD")
			return false
		}
		if err := db.RegisterUser(parts[2], parts[3]); err != nil {
			fmt.Fprintln(os.Stderr, "register:", err)
			return false
		}
		fmt.Fprintln(os.Stderr, "User registered.")
	case "login":
		if len(parts) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: .auth login USERNAME PASSWORD")
			return false
		}
		if err := db.Login(parts[2], parts[3]); err != nil {
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
		fmt.Fprintln(os.Stderr, "Usage: .auth register|login|logout|status [args]")
	}
	return false
}

func runDataCommand(db *gomingleDB.MingleDB, line string) (exit bool) {
	cmd, rest := splitFirstWord(line)
	cmd = strings.ToLower(cmd)
	if rest == "" {
		fmt.Fprintln(os.Stderr, "Missing arguments. Use .help")
		return false
	}

	switch cmd {
	case "insert":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" {
			fmt.Fprintln(os.Stderr, "Usage: insert COLLECTION {json}")
			return false
		}
		doc, err := parseJSON(jsonStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "insert json:", err)
			return false
		}
		if err := db.InsertOne(col, doc); err != nil {
			fmt.Fprintln(os.Stderr, "insert:", err)
			return false
		}
		fmt.Fprintln(os.Stderr, "Inserted 1 document.")
	case "find":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" {
			fmt.Fprintln(os.Stderr, "Usage: find COLLECTION [filter json]")
			return false
		}
		filter := make(map[string]interface{})
		if jsonStr != "" {
			var err error
			filter, err = parseJSONObject(jsonStr)
			if err != nil {
				fmt.Fprintln(os.Stderr, "find filter:", err)
				return false
			}
		}
		docs, err := db.Find(col, filter)
		if err != nil {
			fmt.Fprintln(os.Stderr, "find:", err)
			return false
		}
		printDocs(docs)
	case "findone":
		col, jsonStr := splitCollectionAndJSON(rest)
		if col == "" {
			fmt.Fprintln(os.Stderr, "Usage: findOne COLLECTION [filter json]")
			return false
		}
		filter := make(map[string]interface{})
		if jsonStr != "" {
			var err error
			filter, err = parseJSONObject(jsonStr)
			if err != nil {
				fmt.Fprintln(os.Stderr, "findOne filter:", err)
				return false
			}
		}
		doc, err := db.FindOne(col, filter)
		if err != nil {
			fmt.Fprintln(os.Stderr, "findOne:", err)
			return false
		}
		if doc == nil {
			fmt.Fprintln(os.Stderr, "(no document)")
			return false
		}
		printDocs([]map[string]interface{}{doc})
	case "update":
		col, queryStr, updateStr := splitCollectionQueryUpdate(rest)
		if col == "" || queryStr == "" || updateStr == "" {
			fmt.Fprintln(os.Stderr, "Usage: update COLLECTION query_json update_json")
			return false
		}
		query, err := parseJSONObject(queryStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "update query:", err)
			return false
		}
		update, err := parseJSONObject(updateStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "update update:", err)
			return false
		}
		ok, err := db.UpdateOne(col, query, update)
		if err != nil {
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
			fmt.Fprintln(os.Stderr, "Usage: delete COLLECTION query_json")
			return false
		}
		query, err := parseJSONObject(jsonStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "delete query:", err)
			return false
		}
		ok, err := db.DeleteOne(col, query)
		if err != nil {
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
			fmt.Fprintln(os.Stderr, "Usage: schema COLLECTION schema_json")
			return false
		}
		raw, err := parseJSON(jsonStr)
		if err != nil {
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

func printDocs(docs []map[string]interface{}) {
	for i, doc := range docs {
		b, _ := json.MarshalIndent(doc, "  ", "  ")
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Println(string(b))
	}
	if len(docs) == 0 {
		fmt.Println("(0 documents)")
	} else {
		fmt.Fprintf(os.Stderr, "(%d document(s))\n", len(docs))
	}
}
