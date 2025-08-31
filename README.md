# Gooru

Gooru is a high-performance, content-centric command-line tool for tagging and organizing local files. It uses content hashing to identify files, making it resilient to renames and moves.

This project is currently under development.

## Core Commands

- `gooru add <files...>`: Add files to the database to be tracked.
- `gooru tag <files...> <tags...>`: Add tags to files.
- `gooru untag <files...> [tags...]`: Remove tags from files.
- `gooru list [expression]`: List files matching a tag expression.
- `gooru relinkall <dirs...>`: Scan directories to find moved/renamed files.

## TODO

- Test database performance on a large and a huge db.