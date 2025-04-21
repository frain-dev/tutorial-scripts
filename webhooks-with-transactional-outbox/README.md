# Webhooks with Transactional Outbox Pattern

This tutorial demonstrates how to implement the transactional outbox pattern for reliable webhook delivery using Go, SQLite, and sqlc.

## Prerequisites

- Go 1.21 or later
- sqlc (https://sqlc.dev/docs/install)
- Make

## Project Structure

- `server.go`: Main application with server and worker commands
- `schema.sql`: Database schema and SQL queries
- `sqlc.yaml`: sqlc configuration
- `Makefile`: Build and development commands

## Getting Started

1. Generate the database code:
```bash
make generate
```

2. Build the binary:
```bash
make build
```

3. Run the server (in one terminal):
```bash
./bin/transactional-outbox server
```

4. Run the worker (in another terminal):
```bash
./bin/transactional-outbox worker
```

## Available Commands

- `server`: Run in server mode to accept events via HTTP
- `worker`: Run in worker mode to process events and send webhooks

For more information about a command, use:
```bash
./bin/transactional-outbox <command> --help
```

## Testing

Send a test event to the server:
```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{"business_id": "bus_123", "type": "user.created", "payload": {"user_id": "123", "name": "John Doe"}}'
```

The worker will pick up the event and process it, logging the webhook delivery.

## How It Works

1. The server receives events via HTTP and stores them in the SQLite database
2. The worker continuously polls for pending events
3. When an event is found, the worker:
   - Processes the event (sends webhook)
   - Marks it as processed in the database
4. If the worker fails, the event remains in the database and will be retried

This pattern ensures that no events are lost, even if the worker crashes or the webhook delivery fails. 