# Gooru

Gooru is a high-performance, content-centric command-line tool for tagging and organizing local files. It uses content hashing to identify files, making it resilient to renames and moves.

This project is currently under development.

## Guiding Principles

1.  **Performance First:** All technical decisions prioritize speed, from hashing to database queries. The tool must feel instantaneous and scale to millions of files.
2.  **Clean Architecture:** Logic is strictly separated into distinct layers (CLI, Service, Database) to ensure maintainability and testability.
3.  **Content-Centric:** A file's identity is its content hash. This makes the system resilient to file renames, moves, and duplicates.

## TODO

- Test performance on a large and a huge db.