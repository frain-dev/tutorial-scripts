# Webhooks with Transactional Outbox Pattern

A demonstration of the transactional outbox pattern for reliable webhook delivery using Go, SQLite, and Convoy. This implementation shows how to ensure reliable event processing and webhook delivery even in the face of system failures.

## Overview

This project implements a transactional outbox pattern with two main components:
1. An event ingestion service that generates sample invoice events
2. A worker service that processes events and delivers them via Convoy

The system uses SQLite for persistence and implements proper transaction handling to ensure no events are lost.

## Prerequisites

- Go 1.21 or later
- sqlc (https://sqlc.dev/docs/install)
- Make
- SQLite3
- Convoy account and API credentials

## Project Structure

```
.
├── main.go           # Main application with ingest and worker commands
├── db/
│   ├── schema.sql    # Database schema
│   └── queries.sql   # SQL queries for sqlc
├── sqlc.yaml         # sqlc configuration
├── Makefile          # Build and development commands
└── events.db         # SQLite database (created on first run)
```

## Database Schema

The application uses two main tables:
- `events`: Stores events to be processed
- `invoices`: Stores invoice data that triggers events

## Getting Started

1. Initialize the database:
```bash
make init-db
```

2. Generate the database code:
```bash
make generate
```

3. Build the binary:
```bash
make build
```

4. Run the ingestion service (in one terminal):
```bash
./bin/transactional-outbox ingest --rate 30s
```

5. Run the worker service (in another terminal):
```bash
./bin/transactional-outbox worker \
  --convoy-api-key YOUR_API_KEY \
  --convoy-project-id YOUR_PROJECT_ID \
  --poll-interval 5s
```

## Available Commands

### Ingest Command
```bash
./bin/transactional-outbox ingest [flags]
```
Flags:
- `--rate`: Rate at which to generate events (default: "30s")

### Worker Command
```bash
./bin/transactional-outbox worker [flags]
```
Required Flags:
- `--convoy-api-key`: Your Convoy API key
- `--convoy-project-id`: Your Convoy project ID

Optional Flags:
- `--poll-interval`: Interval at which to poll for events (default: "5s")
- `--convoy-base-url`: Convoy API base URL (default: "https://api.getconvoy.io")

## How It Works

### Event Ingestion
- The ingest service generates sample invoice events at a configurable rate
- Each invoice creation is wrapped in a transaction that:
  1. Creates the invoice record
  2. Creates a corresponding event record
- If either operation fails, the entire transaction is rolled back

### Event Processing
- The worker continuously polls for pending events
- When events are found, it:
  1. Sends them to Convoy for webhook delivery
  2. Marks them as processed in the database
- Failed deliveries are logged but not retried (handled by Convoy)

## Development

To clean up and start fresh:
```bash
make clean
make init-db
make generate
make build
```

## Testing

The ingest service automatically generates sample invoice events. You can monitor the events in the database:

```bash
sqlite3 events.db "SELECT * FROM events ORDER BY created_at DESC LIMIT 5;"
```

## Notes

- The system uses predefined business IDs for demonstration
- Invoice events are generated with random amounts and statuses
- Webhook delivery is handled by Convoy, which provides retry mechanisms and delivery guarantees
- The transactional outbox pattern ensures that no events are lost, even if the worker crashes 