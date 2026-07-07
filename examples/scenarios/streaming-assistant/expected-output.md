# Expected Output

- The stream starts with a `trace` event containing a trace ID.
- One or more `chunk` events carry answer text for incremental rendering.
- A `citations` event lists retrieved evidence.
- A final `done` event tells the client to close the response stream.
