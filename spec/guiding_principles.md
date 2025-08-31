## Guiding Principles

1.  **Performance First:** All technical decisions prioritize speed, from hashing to database queries. The tool must feel instantaneous and scale to millions of files.
2.  **Content-Centric:** A file's identity is its content hash. This makes the system resilient to file renames, moves, and duplicates.
3.  **Clean Architecture:** Logic is strictly separated into distinct layers (CLI, Service, Database) to ensure maintainability and testability.