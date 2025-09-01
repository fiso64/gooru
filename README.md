# Gooru

Gooru is a high-performance, content-centric command-line tool for tagging and organizing local files. It uses content hashing to identify files, making it resilient to renames and moves.

This project is currently under development.

## TODO

- Test database performance on a large and a huge db.

### 1. Saved Query Aliases

Allow users to save complex query expressions under a simple, memorable alias. This alias could then be used in any command that accepts an expression.

*   **Use Case:** A photographer frequently searches for all media files that are not yet part of an archived project. Instead of typing `gooru list "(type:img | type:vid) -project:archive"` repeatedly, they could save it as an alias: `gooru query save unsorted "(type:img | type:vid) -project:archive"`. From then on, they can simply run `gooru list @unsorted` or `gooru tag @unsorted needs_review`.

### 2. Directory-based Tag Inheritance

Automatically apply a common set of tags to any file being added or tagged within a specific directory tree. This could be configured by placing a special file (e.g., `.gooru-tags`) in a directory.

*   **Use Case:** A user organizes their work by client and project (e.g., `/work/client-a/project-x/`). They place a `.gooru-tags` file in `/work/client-a/` containing `client:a`. Now, any file they add from within that directory or its subdirectories, like `gooru add /work/client-a/project-x/brief.pdf`, will automatically be tagged with `client:a`, reducing manual effort and ensuring consistency.

### 3. Interactive Triage Mode

Create a command that iterates through files matching a query, presenting them one by one and prompting the user to apply tags interactively.

*   **Use Case:** A user has a large "inbox" directory of unsorted documents. They run `gooru review "ext:pdf -tagged"`. The command clears the screen, shows the first PDF's path and metadata, and provides a prompt `Tags: `. The user types `invoice client:b`, hits Enter, and the command immediately tags the file and presents the next untagged PDF. This creates a highly efficient workflow for processing a queue of files.

### 4. FUSE Virtual Filesystem

Provide a command to mount the Gooru database as a virtual filesystem, where directories represent tags and files are symlinks to their actual locations. This would allow users to browse their tagged collection using standard GUI file managers.

*   **Use Case:** A user wants to find all photos from their 2023 vacation. Instead of using the command line, they run `gooru mount ~/GooruTags`. They can then open their file manager, navigate to `~/GooruTags/photo/vacation/year:2023`, and see all the relevant photos as if they were organized in that folder, allowing them to use GUI tools like image viewers and slideshow apps directly.