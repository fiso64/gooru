
# Spec: The Gooru gRPC API

**Version:** 1.0
**Status:** Proposed

---

## 1. Abstract

This document specifies a high-performance gRPC API for Gooru, intended to run concurrently with the REST API within the `gooru serve` daemon. This API leverages Protocol Buffers for efficient data serialization and HTTP/2 for advanced transport features like streaming. Its primary purpose is to provide a superior, low-latency experience for dedicated first-party or third-party applications (e.g., GUIs, high-throughput data processing scripts) where performance is critical.

## 2. Problem Statement / Motivation

While the REST API provides universal accessibility, it has inherent limitations for performance-critical applications, especially when dealing with large datasets.

*   **User Story (GUI Developer):** As a GUI developer building a dedicated Gooru front-end, I want to display a list of 100,000 files without the user experiencing lag or "loading" spinners as they scroll, providing a native-app feel.
*   **User Story (Data Scientist):** As a data scientist processing a massive dataset, I need to efficiently stream file information from Gooru into my script, process it, and stream tags back to Gooru without buffering the entire dataset in memory on either the client or the server.

REST with pagination is functional but cannot match the low latency and efficiency of a streaming RPC protocol for these use cases.

## 3. Goals and Non-Goals

### Goals

*   Define a clear, strongly-typed API contract using Protocol Buffers (`.proto` files).
*   Provide streaming RPCs for all operations that can return large result sets (e.g., listing files).
*   Provide bi-directional streaming RPCs for high-throughput batch operations (e.g., tagging millions of files).
*   Ensure the gRPC service can be enabled and run by the `gooru serve` command, co-existing with the REST API.
*   Maximize performance in terms of latency, CPU usage (serialization), and memory footprint.

### Non-Goals

*   This API is not intended for casual, one-off exploration with tools like `curl`. It is designed for programmatic use with gRPC client libraries.
*   It will not replace the REST API, but rather supplement it.

## 4. Proposed Solution & Technical Design

### 4.1. Server Integration

*   The `gooru serve` command will be updated with a new flag: `gooru serve [--grpc-port <port>]`.
*   If the `--grpc-port` flag is provided, the daemon will start a gRPC server on that port in addition to the REST API server.
*   Both servers will share the same underlying Gooru service instance, including the serial write queue, ensuring consistent data handling and concurrency control.

### 4.2. Protocol Buffers Definition (`gooru.v1.proto`)

The API contract will be formally defined in a `.proto` file. This file will be the single source of truth for clients, who will use it to generate their own client-side code.

```proto
syntax = "proto3";

package gooru.v1;

option go_package = "gooru.local/gooru/gen/go/v1;gooruv1";

// --------------------
// Core Data Structures
// --------------------

message FileInfo {
  string path = 1;
  int64 size = 2;
  string tags = 3; // Comma-separated for simplicity, like in the DB
}

message TagWithCount {
  string tag = 1;
  int32 count = 2;
}

// Represents a file source, either by paths or by a query expression.
message FileSource {
  oneof source {
    Paths paths = 1;
    string query = 2;
  }
}

message Paths {
  repeated string path = 1;
}

// Empty message for requests that need no parameters.
message Empty {}

// --------------------
// Gooru Service Definition
// --------------------

service GooruService {
  // --- Read Operations (Streaming) ---

  // Lists files matching a query, streaming results back to the client.
  rpc ListFiles(ListFilesRequest) returns (stream FileInfo);
  // Lists all unique tags in the database.
  rpc ListTags(Empty) returns (stream ListTagsResponse);
  
  // --- Write Operations (Unary & Streaming) ---

  // Adds tags to a set of files.
  rpc TagFiles(TagFilesRequest) returns (TagFilesResponse);
  // Sets (replaces) tags for a set of files.
  rpc SetTags(SetTagsRequest) returns (SetTagsResponse);
  // Removes tags from a set of files.
  rpc Untag(UntagRequest) returns (UntagResponse);
  // Deletes file records from the database.
  rpc DeleteFiles(DeleteFilesRequest) returns (DeleteFilesResponse);

  // High-throughput, bi-directional tagging. The client streams requests
  // to tag individual files, and the server can stream back progress or results.
  rpc StreamTagFiles(stream StreamTagFilesRequest) returns (stream StreamTagFilesResponse);
}

// --- Request/Response Messages ---

message ListFilesRequest {
  string query = 1;
}

message ListTagsResponse {
  oneof result {
    string tag = 1;
    TagWithCount tag_with_count = 2;
  }
}

message TagFilesRequest {
  FileSource source = 1;
  repeated string tags = 2;
}

message TagFilesResponse {
  int64 affected_count = 1;
}

// ... (Similar request/response messages for SetTags, Untag, DeleteFiles) ...

// Messages for the bi-directional stream
message StreamTagFilesRequest {
  string path = 1;
  repeated string tags = 2;
  // Could add an operation type: ADD, SET, REMOVE
}

message StreamTagFilesResponse {
  string path = 1;
  bool success = 2;
  string error_message = 3;
}
```

### 4.3. Key RPC Methods Explained

*   **`rpc ListFiles(ListFilesRequest) returns (stream FileInfo)`**
    *   **Pattern:** Server-side Streaming.
    *   **Behavior:** This is the high-performance equivalent of the `list` and `table` commands. The client sends a single request with a query. The server executes the query once and streams each `FileInfo` result back as it's read from the database.
    *   **Benefit:** Extremely low latency to first result, low memory usage on both client and server, and a consistent view of the data.

*   **`rpc StreamTagFiles(stream StreamTagFilesRequest) returns (stream StreamTagFilesResponse)`**
    *   **Pattern:** Bi-directional Streaming.
    *   **Behavior:** This is a "power user" endpoint for massive batch operations. The client can open a persistent stream and continuously send requests to tag individual files. The server processes these requests (respecting the serial write queue) and can send back real-time status updates for each file.
    *   **Benefit:** Eliminates the overhead of a new HTTP/2 request for every single file in a large batch. This is the most efficient way to perform millions of small, independent write operations.

*   **Unary RPCs (e.g., `TagFiles`)**
    *   **Pattern:** Standard Request/Response.
    *   **Behavior:** These are for simpler batch operations. The client sends one request with a list of files and tags, and the server sends back one response after the operation is complete in the write queue. This is more efficient than the REST equivalent due to Protobufs and HTTP/2, but less efficient than the bi-directional stream for truly massive jobs.

## 5. Edge Cases & Unresolved Questions

*   **Error Handling:** gRPC has a well-defined error model with status codes. Server-side errors (e.g., database unavailable, invalid input) will be propagated back to the client as standard gRPC errors.
*   **Concurrency:** All write RPCs will be funneled through the same serial write queue as the REST API, guaranteeing that a gRPC `TagFiles` call will not conflict with a REST `DeleteFiles` call. They will be executed in the order they are received by the server.
*   **API Versioning:** The API is versioned in its package name (`gooru.v1`) and file path. This allows for future, non-backwards-compatible changes by introducing a `v2` package.

## 6. Alternatives Considered

*   **Sticking with REST only:** As discussed, this is a viable but less performant option. It fails to provide the best possible experience for dedicated, data-intensive clients. By offering both, we cater to all use cases.
*   **Twirp:** A protocol from Twitch that provides a Protobuf-based API but works over standard HTTP/1.1, making it compatible with more infrastructure. However, it loses the key benefits of HTTP/2 multiplexing and streaming, which are the primary motivators for adopting a new protocol here. gRPC is the better technical choice for this specific performance goal.