# Flow Diagras

**Version:** 1.0
**Status:** Idea

---

Diagrams for the existing or future application flows.

### Diagram 1: Standalone CLI Usage (No Server)

This shows a user interacting directly with Gooru from their terminal.

```
+-----------------------------------------------------------------+
|                        User's Machine                           |
|                                                                 |
|   +---------+        +--------------+                           |
|   | 👤 User | -----> |   Terminal   |                           |
|   +---------+        +--------------+                           |
|                          |           |                          |
|  +-----------------------+           +------------------------+ |
|  | A) Short-Lived Command            | B) Long-Lived Command  | |
|  |                                   |                        | |
|  | "gooru tag file.txt mytag"        | "gooru mount ./view"   | |
|  |                                   |                        | |
|  v                                   v                        | |
| +-------------------------+         +--------------------------+ |
| |   Gooru CLI Process     |         |  Gooru Mount Process     | |
| |   (Starts & Exits)      |         |  (Starts & Persists)     | |
| +-------------------------+         +--------------------------+ |
|      |            |                      |           |          |
|      |            |                      |           |          |
|      v            v                      v           v          |
| +------------+ +-----------+          +-----------+ +------------+ |
| | (Database) | |(Filesystem)|         |(Database) | |(VFS/FUSE)  | |
| +------------+ +-----------+          +-----------+ +-----.------+ |
|                                                             |      |
|                                                             '------> (Filesystem)
+-----------------------------------------------------------------+
```

---

### Diagram 2: API Server Handling a Short-Lived Request (e.g., `tag`)

This shows the "manager is the worker" scenario for stateless API calls.

```
+-----------------------------------------------------------------------------+
|                               User's Machine                                |
|                                                                             |
| +------------------+              +---------------------------------------+ |
| | External App     |              |      gooru serve Process              | |
| | (Python, GUI...) |              |      (Manager/Worker)                 | |
| +------------------+              | +-----------------------------------+ | |
|         |                         | | +----------------+                | | |
|         | POST /api/v1/files/tags | | | HTTP Endpoint  | ---------------> | | |
|         +-------------------------> | +----------------+   (Internal)   | | |
|                                   |                      |              | | |
|                                   |                      v              | | |
|                                   |             +---------------------+ | | |
|                                   |             | Gooru Business Logic| | | |
|                                   |             +---------------------+ | | |
|                                   |                  |         |        | | |
|                                   |                  v         v        | | |
|                                   |           +----------+ +---------+ | | |
|                                   |           | (DB)     | | (FS)    | | | |
|                                   |           +----------+ +---------+ | | |
|         <-------------------------+                                   | | |
|             200 OK (Response)     | +-----------------------------------+ | |
|                                   +---------------------------------------+ |
+-----------------------------------------------------------------------------+
```

---

### Diagram 3: API Server Handling a Long-Lived Request (e.g., `mount`)

This shows the "manager delegates to a worker" scenario for stateful, persistent tasks.

```
+-----------------------------------------------------------------------------------+
|                                 User's Machine                                    |
|                                                                                   |
| +--------------+      +-------------------+      +--------------------------------+ |
| | External App |      | Manager Process   |      | Worker Process                 | |
| +--------------+      | (gooru serve)     |      | (gooru mount)                  | |
|       |               +-------------------+      +--------------------------------+ |
|       | POST /mounts            |                            |                    | |
|       +-----------------------> |                            |                    | |
|                                 | 2. Spawns Process          |                    | |
|                                 +--------------------------->|                    | |
|       <-------------------------+                            |                    | |
|        3. 201 Created           |                            v                    | |
|           (Response)            |                      (Runs in background,       | |
|                                 |                       creates VFS, etc.)        | |
|                                 |                                                 | |
+-----------------------------------------------------------------------------------+
```

---

### Diagram 4: Complete API Control Flow

This diagram shows the full architecture, including how the Manager uses a private socket to control the Worker process it spawned.

```
+-----------------------------------------------------------------------------------+
|                                     Gooru System                                  |
|                                                                                   |
| +--------------+      +-------------------+      +--------------------------------+ |
| | External App |      | Manager Process   |      | Worker Process (mount_id: xyz) | |
| | (Public API  |      | (gooru serve)     |      | (gooru mount)                  | |
| |  Consumer)   |      +-------------------+      +--------------------------------+ |
| +--------------+               ^                          ^                       | |
|       |                        |                          |                       | |
|       |                        |<-------------------------+                       | |
|       |                        |   Private IPC Socket     |                       | |
|       |                        | (Implementation Detail)  |                       | |
|       |                        +------------------------->|                       | |
|       |                                                   |                       | |
|   A) POST /files/tags (Short-Lived)                       |                       | |
|       +----------------------->| (Handled internally)     |                       | |
|                                                           |                       | |
|   B) POST /mounts (Long-Lived)                            |                       | |
|       +----------------------->| -- (Spawns) ------------>|                       | |
|                                                           |                       | |
|   C) PATCH /mounts/xyz (Control)                          |                       | |
|       +----------------------->| -- "SET_QUERY..." ------>| (Via Socket)          | |
|                                                                                   | |
+-----------------------------------------------------------------------------------+
```

**Explanation of Flow C:**
1.  The external app sends a standard `PATCH` HTTP request to the Manager.
2.  The Manager receives the request and identifies the target mount (`xyz`).
3.  The Manager uses its internal, private socket to send a simple command (`SET_QUERY...`) to the correct Worker.
4.  The Worker receives the command from the socket and updates its FUSE view.
5.  The external app only ever speaks HTTP; the socket is a hidden implementation detail.