# mingledb-cli

Interactive SQLite-style shell for [mingleDB](https://github.com/marcuwynu23/mingledb) / [gomingleDB](../gomingleDB) (`.mgdb`) databases.

## Build

Requires [gomingleDB](../gomingleDB) in the parent directory.

```bash
go build -o mingledb .
```

## Usage

```bash
./mingledb [PATH]   # PATH = database directory (default: ./mydb)
```

## Dot commands (like SQLite)

| Command | Description |
|---------|-------------|
| `.exit`, `.quit` | Exit the CLI |
| `.help` | Show all commands |
| `.databases` | Show current database directory |
| `.open PATH` | Open another database directory |
| `.collections`, `.tables` | List collection names |
| `.schema [NAME]` | Show schema for collection (or list collections with schema) |
| `.auth register USER PASS` | Register a user |
| `.auth login USER PASS` | Log in |
| `.auth logout` | Log out |
| `.auth status` | Show current user |

## Data commands

- **insert** `COLLECTION` `{json}` — Insert one document
- **find** `COLLECTION` `[filter json]` — Find documents (empty filter = all)
- **findOne** `COLLECTION` `[filter json]` — First matching document
- **update** `COLLECTION` `query_json` `update_json` — Update one matching document
- **delete** `COLLECTION` `query_json` — Delete one matching document
- **schema** `COLLECTION` `schema_json` — Define schema (e.g. `{"name":{"type":"string","required":true}}`)

Query operators: `$gt`, `$gte`, `$lt`, `$lte`, `$eq`, `$ne`, `$in`, `$nin`, `$regex` (with optional `$options`).

## Example

```
mingledb> .databases
D:\data\mydb

mingledb> insert users {"name":"Alice","email":"a@b.com","age":30}
Inserted 1 document.

mingledb> find users {"age":{"$gte":18}}
  {
    "age": 30,
    "email": "a@b.com",
    "name": "Alice"
  }
(1 document(s))

mingledb> .exit
Bye.
```
