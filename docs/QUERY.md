# Query language

Gooru's query language is used by `list`, `table`, `count`, `exists`, query-based tag mutations, saved searches, mounts, and the web API.

## Operators

| Meaning | Syntax | Example |
| --- | --- | --- |
| AND | whitespace or `&` | `photo favorite` |
| OR | `|` or the word `or` | `photo | video` |
| NOT | `-`, `!`, or the word `not` | `photo -work` |
| Grouping | `( … )` | `(photo | video) favorite` |

AND binds within an OR branch. Parentheses are recommended whenever a mixed expression could be read ambiguously.

```text
photo favorite
photo & favorite
photo and favorite
```

All three forms mean the same thing.

## Tags and namespaces

A simple tag is a key with an empty value:

```text
favorite
```

A namespaced tag has a key and value:

```text
event:wedding
person:alice
rating:5
```

### Key-only matching

A bare key matches both the simple tag and any `key:value` tag using that key.

```text
location
```

matches `location`, `location:zurich`, and `location:home`.

Use these forms when you need to distinguish them:

```text
location:    # the simple/empty-value tag only
location:*   # any non-empty value for the location key
```

## File extension and type

`ext:` is a virtual query namespace for file extensions:

```text
ext:jpg
ext:mp4 | ext:mov
```

`type:` expands common media types:

```text
type:img
type:vid
```

`ext` and `type` are reserved query keys and cannot be created as normal tag keys.

## Meta-queries

### Files with or without tags

```text
@tagged
-@tagged
```

### Filename matching

```text
@filename_contains:scan
```

For a value containing spaces, quote the complete token:

```text
"@filename_contains:summer trip"
```

Filename matching is implemented using SQLite `LIKE` against stored paths and may be more scan-heavy than indexed tag queries on large libraries.

## Examples

```text
photo trip:iceland
(photo | video) trip:iceland
photo -private
(type:img | type:vid) favorite
location:* -location:home
@tagged -archive
"@filename_contains:invoice 2025"
```

## Tag syntax

Tags may contain printable non-space ASCII characters, subject to validation rules. Keys and values cannot start or end with `-`, `!`, or `:`; the key cannot be empty.

Examples:

```text
photo
project:alpha
version-1.0
needs_review
```

Invalid examples include tags containing spaces, `:work`, or values ending in `:`.
