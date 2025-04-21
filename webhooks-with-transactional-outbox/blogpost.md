# Transactional Outbox: How to reliably generate webhook events

In the world of distributed systems, ensuring reliable event delivery is crucial, especially when dealing with webhooks. The transactional outbox pattern has emerged as a robust solution to this challenge. In this post, we'll explore how to implement this pattern to guarantee reliable webhook delivery, even in the face of system failures.

## Introduction

When building systems that need to notify external services about events (webhooks), we face a fundamental challenge: how do we ensure that every event is delivered exactly once, even when our system experiences failures? The transactional outbox pattern solves this by treating the event publication as part of the same transaction as the business operation.

The key benefits of this pattern are:
- Atomic operations: Events are stored in the same transaction as the business data
- Guaranteed delivery: No events are lost, even if the system crashes
- Exactly-once delivery: Events are processed only once
- Scalability: The pattern works well with high-throughput systems

## Designing the Outbox

Let's dive into implementing the transactional outbox pattern using Go, SQLite, and sqlc. Our implementation consists of two main components: an ingest service that creates events and a worker that processes them.

### Database Schema

First, we need to design our database schema to store both our business data and events:

```sql
-- Events table for storing webhook events
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    business_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME,
    status TEXT DEFAULT 'pending'
);

-- Business data table (invoices in our example)
CREATE TABLE IF NOT EXISTS invoices (
    id TEXT PRIMARY KEY,
    business_id TEXT NOT NULL,
    amount REAL NOT NULL,
    currency TEXT NOT NULL,
    status TEXT NOT NULL,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Ingest Service

The ingest service is responsible for creating business objects and their associated events in a single transaction. Here's how it works:

```go
func runIngest(queries *db.Queries, dbConn *sql.DB, rate time.Duration) error {
    // Start a transaction
    tx, err := dbConn.BeginTx(context.Background(), nil)
    if err != nil {
        return err
    }

    // Create a new queries instance that uses the transaction
    txQueries := queries.WithTx(tx)

    // Create the invoice within the transaction
    _, err = txQueries.CreateInvoice(context.Background(), db.CreateInvoiceParams{
        ID:          invoice.ID,
        BusinessID:  invoice.BusinessID,
        Amount:      invoice.Amount,
        Currency:    invoice.Currency,
        Status:      invoice.Status,
        Description: sql.NullString{String: invoice.Description, Valid: true},
    })

    // Create the event within the same transaction
    _, err = txQueries.CreateEvent(context.Background(), db.CreateEventParams{
        BusinessID: businessID,
        EventType:  "invoice.created",
        Payload:    string(payload),
    })

    // Commit the transaction
    return tx.Commit()
}
```

The key here is that both the invoice creation and event creation happen in the same transaction. If either operation fails, the entire transaction is rolled back, ensuring data consistency.

### Worker Service

The worker service is responsible for processing pending events and sending webhooks. It runs continuously, polling for new events:

```go
func runWorker(queries *db.Queries, dbConn *sql.DB, pollInterval time.Duration, convoyClient *convoy.Client) error {
    for {
        // Fetch pending events
        events, err := queries.GetPendingEvents(context.Background(), batchSize)
        if err != nil {
            log.Printf("Error fetching events: %v", err)
            time.Sleep(pollInterval)
            continue
        }

        for _, event := range events {
            // Create a fanout event using Convoy
            fanoutEvent := &convoy.CreateFanoutEventRequest{
                EventType:      event.EventType,
                OwnerID:        event.BusinessID,
                IdempotencyKey: event.ID,
                Data:           []byte(event.Payload),
            }

            // Send the event to Convoy
            err = convoyClient.Events.FanoutEvent(context.Background(), fanoutEvent)
            if err != nil {
                log.Printf("Error sending event %d to Convoy: %v", event.ID, err)
                continue
            }

            // Mark event as processed
            if err := queries.MarkEventAsProcessed(context.Background(), event.ID); err != nil {
                log.Printf("Error marking event %d as processed: %v", event.ID, err)
                continue
            }
        }

        time.Sleep(pollInterval)
    }
}
```

The worker uses idempotency keys to ensure exactly-once delivery, and it only marks events as processed after successful webhook delivery.

## Operational Tips

When running a transactional outbox system in production, consider these important operational aspects:

1. **Database Performance**
   - Create appropriate indexes on the events table (business_id, status, created_at)
   - Monitor the size of the events table and implement a cleanup strategy for processed events
   - Consider partitioning the events table by date if dealing with high volume

2. **Worker Configuration**
   - Set appropriate batch sizes based on your system's capacity
   - Configure reasonable poll intervals to balance latency and database load
   - Use multiple worker instances for horizontal scaling

3. **Monitoring and Alerting**
   - Monitor the number of pending events
   - Track event processing success rates
   - Alert on high failure rates or processing delays
   - Set up logging for debugging event processing issues

4. **Error Handling**
   - Implement dead letter queues for events that fail after multiple retries
   - Set up monitoring for stuck events (events that haven't been processed for too long)
   - Have a process for manually retrying failed events when necessary

5. **Scaling Considerations**
   - Use database connection pooling
   - Consider using a more robust database like PostgreSQL for production
   - Implement rate limiting for event processing
   - Use a message queue for the worker to handle high throughput

By following these operational guidelines and implementing the transactional outbox pattern as shown, you can build a reliable event processing system that guarantees exactly-once delivery, even in the face of system failures. 