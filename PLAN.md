
### **Project `gooru`: Backend CLI Plan (Revised)**

This plan outlines the INITIAL architecture and components for a high-performance, file-tagging command-line tool.

#### **Guiding Principles**

1.  **Performance First:** All technical decisions will prioritize speed, from hashing to database queries. The tool must feel instantaneous.
2.  **Clean Architecture:** Logic will be strictly separated into distinct layers (CLI, Service, Database) to ensure maintainability and testability.
3.  **Content-Centric:** A file's identity is its content hash. This makes the system resilient to file renames, moves, and duplicates.

---

### **Phase 1: The Foundation (Core Structure & Data Model)**

This phase establishes the project's non-negotiable core.

*   **Project Structure:** A standard Go project layout will be used to separate concerns, with distinct packages for the CLI entrypoint (`cmd`), core business logic (`internal/service`), database interaction (`internal/database`), hashing, and query parsing.

*   **Hashing Strategy:** To achieve maximum speed, the primary file identifier will be generated using a high-throughput hashing algorithm (e.g., BLAKE3). The goal is to be limited by disk I/O, not CPU, during hashing operations.

*   **Database Schema (SQLite):** The schema is designed for query speed and data integrity.
    *   `contents`: Stores unique file hashes (`hash` as PRIMARY KEY) and metadata.
    *   `locations`: Maps content hashes to their current absolute file paths on disk.
    *   `tags`: Stores unique tag names.
    *   `content_tags`: A join table linking content hashes to tags, forming the core relationship.
    *   **Critical Feature:** Proper indexing will be applied to all foreign keys and frequently searched columns to ensure fast lookups.

---

### **Phase 2: The Core Logic (Service Layer)**

This phase creates an internal API that orchestrates operations, completely independent of the command-line interface.

*   **`database` Package:** This package will be the sole component responsible for all SQL operations. It will provide a clean Go API to the service layer for actions like adding content, managing tags, and executing complex queries.

*   **`service` Package:** This is the application's brain. It will consume functionality from the `database` and `hashing` packages to provide the core features:
    *   **Tagging/Untagging Files:** Logic to hash files, find or create database records, and associate/disassociate tags with file content.
    *   **Retrieving Tags:** Logic to find the tags associated with a specific file path.
    *   **Listing Files:** Logic that coordinates with the query parsing layer to fetch files matching a boolean expression.

---

### **Phase 3: The Command-Line Interface**

This phase exposes the core service logic to the user via a polished and intuitive CLI, built using a standard Go CLI framework.

*   **Command: `gooru tag`**
    *   **Single-File (Default):** `gooru tag <filepath> <tag1> [tag2...]`
        *   The first argument is the file; all subsequent arguments are tags.
    *   **Multi-File (Explicit):** `gooru tag -m/--multi <file1> [file2...] -- <tag1> [tag2...]`
        *   Requires the `-m` flag. The `--` separator is mandatory to distinguish the file list from the tag list.

*   **Command: `gooru untag`**
    *   Follows the exact same syntax and logic as the `tag` command.

*   **Command: `gooru gettags <filepath>`**
    *   **Action:** Takes a single file path as an argument.
    *   **Output:** Hashes the file, looks up its content in the database, and prints a simple list of its associated tags.

*   **Command: `gooru list [expression]`**
    *   **Action:** Lists all file paths that match the given boolean expression.
    *   **Examples:**
        *   `gooru list` → Lists all files known to the system.
        *   `gooru list photos` → Lists all files tagged with `photos`.
        *   `gooru list "(photos OR vacation) AND NOT work"` → Lists files matching the complex query.

---

### **Phase 4: Advanced Query Expression Parsing**

This phase delivers the powerful `list` command functionality.

*   **Goal:** To translate a user-friendly expression string into a single, highly efficient SQL query.

*   **Strategy:** A dedicated parser will be built to analyze the expression's structure (parentheses, AND, OR, NOT, and tag names). It will then dynamically construct a SQL query that uses set operations (`UNION`, `INTERSECT`, `EXCEPT`). This approach delegates the complex logical filtering to the optimized SQLite query engine, which is vastly more performant than processing these operations in Go. The final result from the database will be the precise list of content hashes matching the user's criteria.