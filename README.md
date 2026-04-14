# MingleDB CLI

**An interactive shell for MingleDB.**  
Connect to a `.mgdb` database file (or a directory that resolves to one file) and run queries, manage collections, and handle auth from the terminal.

---

## Overview

MingleDB CLI is the official command-line interface for [MingleDB](https://github.com/mingledb/mingledb) and [gomingleDB](https://github.com/mingledb/gomingleDB). It provides a familiar, REPL-style experience similar to the SQLite shell: dot commands for meta-operations and data commands for CRUD.

| Feature          | Description                                                                |
| ---------------- | -------------------------------------------------------------------------- |
| **Dot commands** | `.exit`, `.databases`, `.collections`, `.tables`, `.schema`, `.open`, `.auth`, `.system`, `.output` |
| **CRUD**         | `insert`, `find`, `findOne`, `update`, `delete` with JSON docs and filters |
| **Schema**       | Define and inspect collection schemas from the shell                       |
| **Auth**         | Register, login, logout, and check session status                          |
| **Portable**     | Single binary; works with any mingleDB/gomingleDB `.mgdb` single-file database |

---

## Prerequisites

- **Go 1.21+**
- Access to the `github.com/mingledb/gomingleDB` module (fetched automatically by Go)

---

## Installation

Clone this repo, then build:

```bash
cd mingledb-cli
go build -o mgdb .
```

On Windows:

```powershell
go build -o mgdb.exe .
```

The binary is self-contained; you can move it anywhere and run it against any mingleDB database file.

---

## Quick Start

Open a database (starts in in-memory mode if no path is given):

```bash
./mgdb                          # in-memory database
./mgdb /path/to/app.mgdb        # explicit database file
./mgdb /path/to/data            # directory -> /path/to/data/database.mgdb
```

Then at the prompt:

```text
mingledb> .help
mingledb> .databases
mingledb> insert users {"name":"Alice","email":"alice@example.com","age":30}
mingledb> find users
mingledb> .exit
```

---

## Dot Commands

All meta-commands start with a dot (`.`). They control the session, current database, and auth.

| Command                    | Description                                                                                        |
| -------------------------- | -------------------------------------------------------------------------------------------------- |
| `.exit`                    | Exit the shell (alias: `.quit`)                                                                    |
| `.help`                    | Print help and command reference                                                                   |
| `.databases`               | Print current database path (or `(memory)` if in-memory)                                          |
| `.open PATH`               | Switch to another database path (`.mgdb` file or directory)                                       |
| `.collections`             | List all collection names                                                                          |
| `.tables`                  | Alias for `.collections`                                                                           |
| `.schema [NAME]`           | With `NAME`: show schema for that collection. Without: list collections that have a schema defined |
| `.auth register USER PASS` | Register a new user                                                                                |
| `.auth login USER PASS`    | Log in as a user                                                                                   |
| `.auth logout`             | End the current session                                                                            |
| `.auth status`             | Show the currently logged-in user (or "not logged in")                                             |
| `.system CMD [args...]`    | Run a system command (e.g. `.system ls -la`, `.system dir`)                                        |
| `.output PATH`             | Save in-memory database to disk path (`.mgdb` file or directory)                                  |

---

## Data Commands

Arguments are **collection name** and **JSON** (document, filter, or schema). JSON can be inline; multi-line input is supported by ending a line with `\` and continuing on the next.

### Insert

Insert one document into a collection.

```text
insert <collection> <document>
```

**Example:**

```text
mingledb> insert users {"name":"Alice","email":"alice@example.com","age":30}
Inserted 1 document.
```

---

### Find

Return all documents that match an optional filter. Omit the filter to return every document.

```text
find <collection> [filter]
```

**Examples:**

```text
mingledb> find users
mingledb> find users {"age":{"$gte":18,"$lt":60}}
mingledb> find users {"email":{"$in":["a@b.com","c@d.com"]}}
```

---

### FindOne

Return the first document that matches an optional filter.

```text
findOne <collection> [filter]
```

**Example:**

```text
mingledb> findOne users {"email":"alice@example.com"}
```

---

### Update

Update the first document that matches the query with the given fields.

```text
update <collection> <query> <update>
```

**Example:**

```text
mingledb> update users {"email":"alice@example.com"} {"age":31}
Updated 1 document.
```

---

### Delete

Remove the first document that matches the query.

```text
delete <collection> <query>
```

**Example:**

```text
mingledb> delete users {"email":"alice@example.com"}
Deleted 1 document.
```

---

### Schema

Define a schema for a collection (field types, required, unique).

```text
schema <collection> <schema_json>
```

**Example:**

```text
mingledb> schema users {"name":{"type":"string","required":true},"email":{"type":"string","required":true,"unique":true},"age":{"type":"number"}}
Schema defined for users
```

---

## Query Operators

Use these inside filter/query JSON for `find`, `findOne`, `update`, and `delete`:

| Operator | Meaning               | Example                                     |
| -------- | --------------------- | ------------------------------------------- |
| `$gt`    | Greater than          | `{"age":{"$gt":18}}`                        |
| `$gte`   | Greater than or equal | `{"age":{"$gte":18}}`                       |
| `$lt`    | Less than             | `{"age":{"$lt":65}}`                        |
| `$lte`   | Less than or equal    | `{"age":{"$lte":65}}`                       |
| `$eq`    | Equal                 | `{"status":{"$eq":"active"}}`               |
| `$ne`    | Not equal             | `{"status":{"$ne":"archived"}}`             |
| `$in`    | Value in list         | `{"role":{"$in":["admin","editor"]}}`       |
| `$nin`   | Value not in list     | `{"role":{"$nin":["banned"]}}`              |
| `$regex` | Regular expression    | `{"name":{"$regex":"^Ali","$options":"i"}}` |

You can combine operators (e.g. `{"age":{"$gte":18,"$lt":60}}`).

---

## Example Session

```text
$ ./mgdb ./data

mgdb 1.0
Type .help for commands.

mingledb> .databases
/path/to/data

mingledb> .collections
users

mingledb> .tables
users

mingledb> insert users {"name":"Alice","email":"alice@example.com","age":30}
Inserted 1 document.

mingledb> find users {"age":{"$gte":18}}
  {
    "age": 30,
    "email": "alice@example.com",
    "name": "Alice"
  }
(1 document(s))

mingledb> update users {"email":"alice@example.com"} {"age":31}
Updated 1 document.

mingledb> findOne users {"email":"alice@example.com"}
  {
    "age": 31,
    "email": "alice@example.com",
    "name": "Alice"
  }
(1 document(s))

mingledb> .exit
Bye.
```

---

## Related

- [MingleDB](https://github.com/mingledb/mingledb) — Node.js implementation
- [gomingleDB](https://github.com/mingledb/gomingleDB) — Go implementation (used by this CLI)

---

## Community Standards

- Contribution guide: [`CONTRIBUTING.md`](./CONTRIBUTING.md)
- Code of conduct: [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md)
- Changelog: [`CHANGELOG.md`](./CHANGELOG.md)
- Publishing guide: [`PUBLISH.md`](./PUBLISH.md)
- License: [`LICENSE`](./LICENSE)
- Funding: [`.github/FUNDING.yml`](./.github/FUNDING.yml)
- Bug reports and feature requests: [Issue templates](./.github/ISSUE_TEMPLATE/)
- Pull requests: [PR template](./.github/pull_request_template.md)

---

## License

MIT
