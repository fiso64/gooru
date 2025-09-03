# Gooru

Gooru is a high-performance, content-centric command-line tool for tagging and organizing local files. It uses content hashing to identify files, making it resilient to renames and moves.

This project is currently under development.

## TODO

- Priority #1: Extensive tests to ensure correctness of all operations.
- Test database performance on a large and a huge db. (potential optimization: FTS)

### 1. Saved Query Aliases

Allow users to save complex query expressions under a simple, memorable alias. This alias could then be used in any command that accepts an expression.

*   **Use Case:** A photographer frequently searches for all media files that are not yet part of an archived project. Instead of typing `gooru list "(type:img | type:vid) -project:archive"` repeatedly, they could save it as an alias: `gooru query save unsorted "(type:img | type:vid) -project:archive"`. From then on, they can simply run `gooru list @unsorted` or `gooru tag @unsorted needs_review`.

### 2. Directory-based Tag Inheritance

Automatically apply a common set of tags to any file being added or tagged within a specific directory tree. This could be configured by placing a special file (e.g., `.gooru-tags`) in a directory, or in a global config file.

*   **Use Case:** A user organizes their work by client and project (e.g., `/work/client-a/project-x/`). They place a `.gooru-tags` file in `/work/client-a/` containing `client:a`. Now, any file they add from within that directory or its subdirectories, like `gooru add /work/client-a/project-x/brief.pdf`, will automatically be tagged with `client:a`, reducing manual effort and ensuring consistency.
*   Question/Problem: Implemented as above, a new file in a directory with associated auto-tags will not automatically obtain these tags until user `add`s it. Confusing/unintuitive?