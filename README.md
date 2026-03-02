# MingleDB CLI

**A interactive shell for MingleDB.**  
Connect to any `.mgdb` database directory and run queries, manage collections, and handle auth from the terminal.

---

## Overview

MingleDB CLI is the official command-line interface for [MingleDB](https://github.com/marcuwynu23/mingledb) and [gomingleDB](../gomingleDB). It provides a familiar, REPL-style experience similar to the SQLite shell: dot commands for meta-operations and data commands for CRUD.

| Feature          | Description                                                                |
| ---------------- | -------------------------------------------------------------------------- |
| **Dot commands** | `.exit`, `.databases`, `.collections`, `.schema`, `.open`, `.auth`         |
| **CRUD**         | `insert`, `find`, `findOne`, `update`, `delete` with JSON docs and filters |
| **Schema**       | Define and inspect collection schemas from the shell                       |
| **Auth**         | Register, login, logout, and check session status                          |
| **Portable**     | Single binary; works with any mingleDB/gomingleDB `.mgdb` directory        |

---

## Prerequisites

- **Go 1.21+**
- **gomingleDB** in the parent directory (e.g. `../gomingleDB`) for building the CLI

---

## Installation

Clone or place this repo next to [gomingleDB](../gomingleDB), then build:

```bash
cd mingledb-cli
go build -o mingledb .
```

On Windows:

```powershell
go build -o mingledb.exe .
```

The binary is self-contained; you can move it anywhere and run it against any mingleDB database directory.

---

## Quick Start

Open a database (default directory is `./mydb` if no path is given):

```bash
./mingledb                    # use ./mydb
./mingledb /path/to/db        # use specified directory
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
| `.databases`               | Print the current database directory path                                                          |
| `.open PATH`               | Switch to another database directory                                                               |
| `.collections`             | List all collection names (alias: `.tables`)                                                       |
| `.schema [NAME]`           | With `NAME`: show schema for that collection. Without: list collections that have a schema defined |
| `.auth register USER PASS` | Register a new user                                                                                |
| `.auth login USER PASS`    | Log in as a user                                                                                   |
| `.auth logout`             | End the current session                                                                            |
| `.auth status`             | Show the currently logged-in user (or “not logged in”)                                             |

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
$ ./mingledb ./data

mingledb-cli 1.0
Type .help for commands.

mingledb> .databases
/path/to/data

mingledb> .collections
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

- [MingleDB](https://github.com/marcuwynu23/mingledb) — Node.js implementation
- [gomingleDB](../gomingleDB) — Go implementation (used by this CLI)

---

## License

MIT
