package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	convoy "github.com/frain-dev/convoy-go/v2"
	"github.com/frain-dev/webhooks-with-transactional-outbox/db"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

// Predefined business IDs with UUIDs
var businessIDs = []string{
	"550e8400-e29b-41d4-a716-446655440000", // Acme Corp
	"6ba7b810-9dad-11d1-80b4-00c04fd430c8", // TechStart Inc
	"7ba7b810-9dad-11d1-80b4-00c04fd430c9", // Global Solutions
	"8ba7b810-9dad-11d1-80b4-00c04fd430ca", // Innovate Labs
	"9ba7b810-9dad-11d1-80b4-00c04fd430cb", // Future Systems
}

// getRandomBusinessID returns a random business ID from the predefined list
func getRandomBusinessID() string {
	return businessIDs[rand.Intn(len(businessIDs))]
}

type Invoice struct {
	ID          string    `json:"id"`
	BusinessID  string    `json:"business_id"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	Description string    `json:"description"`
}

type Event struct {
	BusinessID string          `json:"business_id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
}

const (
	batchSize = 10
)

func generateInvoice(businessID string) Invoice {
	currencies := []string{"USD", "EUR", "GBP"}
	statuses := []string{"draft", "sent", "paid", "overdue"}

	return Invoice{
		ID:          fmt.Sprintf("INV-%d", rand.Intn(1000000)),
		BusinessID:  businessID,
		Amount:      float64(rand.Intn(10000)) + 99.99,
		Currency:    currencies[rand.Intn(len(currencies))],
		Status:      statuses[rand.Intn(len(statuses))],
		CreatedAt:   time.Now(),
		Description: "Sample invoice for demonstration",
	}
}

func runIngest(queries *db.Queries, dbConn *sql.DB, rate time.Duration) error {
	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	for range ticker.C {
		// Get a random business ID from our predefined list
		businessID := getRandomBusinessID()

		// Generate an invoice
		invoice := generateInvoice(businessID)

		// Start a transaction
		tx, err := dbConn.BeginTx(context.Background(), nil)
		if err != nil {
			log.Printf("Error starting transaction: %v", err)
			continue
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
		if err != nil {
			tx.Rollback()
			log.Printf("Error creating invoice: %v", err)
			continue
		}

		// Marshal the invoice for the event payload
		eventPayload := struct {
			EventType string      `json:"event_type"`
			Data      interface{} `json:"data"`
		}{
			EventType: "invoice.created",
			Data:      invoice,
		}
		payload, err := json.Marshal(eventPayload)
		if err != nil {
			tx.Rollback()
			log.Printf("Error marshaling invoice: %v", err)
			continue
		}

		// Create the event within the same transaction
		_, err = txQueries.CreateEvent(context.Background(), db.CreateEventParams{
			BusinessID: businessID,
			EventType:  "invoice.created",
			Payload:    string(payload),
		})
		if err != nil {
			tx.Rollback()
			log.Printf("Error creating event: %v", err)
			continue
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			log.Printf("Error committing transaction: %v", err)
			continue
		}

		log.Printf("Created invoice and event for business %s: %s", businessID, string(payload))
	}

	return nil
}

func runWorker(queries *db.Queries, dbConn *sql.DB, pollInterval time.Duration, convoyClient *convoy.Client) error {
	for {
		events, err := queries.GetPendingEvents(context.Background(), batchSize)
		if err != nil {
			log.Printf("Error fetching events: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		if len(events) == 0 {
			log.Printf("No pending events found. Polling again in %v", pollInterval)
			time.Sleep(pollInterval)
			continue
		}

		log.Printf("Found %d pending events to process", len(events))

		for _, event := range events {

			// Ensure payload is not empty
			if event.Payload == "" {
				log.Printf("Warning: Empty payload for event %d, skipping", event.ID)
				continue
			}

			// Create a fanout event using Convoy
			fanoutEvent := &convoy.CreateFanoutEventRequest{
				EventType:      event.EventType,
				OwnerID:        event.BusinessID, // Using business_id as owner_id
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

func getDB() (*db.Queries, *sql.DB, error) {
	dbConn, err := sql.Open("sqlite3", "events.db")
	if err != nil {
		return nil, nil, err
	}
	return db.New(dbConn), dbConn, nil
}

// initDB initializes the database with the schema if it doesn't exist
func initDB(dbPath string) error {
	// Check if database file exists
	if _, err := os.Stat(dbPath); err == nil {
		fmt.Printf("Database file %s already exists. Do you want to recreate it? (y/n): ", dbPath)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Using existing database.")
			return nil
		}
		// Remove existing database file
		if err := os.Remove(dbPath); err != nil {
			return fmt.Errorf("error removing existing database: %v", err)
		}
	}

	// Open database connection
	dbConn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("error opening database: %v", err)
	}
	defer dbConn.Close()

	// Read schema.sql file
	schemaPath := filepath.Join("db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("error reading schema file: %v", err)
	}

	// Execute schema SQL
	_, err = dbConn.Exec(string(schemaSQL))
	if err != nil {
		return fmt.Errorf("error executing schema: %v", err)
	}

	fmt.Println("Database initialized successfully!")
	return nil
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "transactional-outbox",
		Short: "Transactional outbox pattern implementation for webhook delivery",
		Long: `A demonstration of the transactional outbox pattern for reliable webhook delivery.
This application can run in either ingest mode to generate events or worker mode to process them.`,
	}

	// Initialize database on startup
	dbPath := "events.db"
	if err := initDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	var rate string
	var ingestCmd = &cobra.Command{
		Use:   "ingest",
		Short: "Run in ingest mode to generate invoice events",
		RunE: func(cmd *cobra.Command, args []string) error {
			rateDuration, err := time.ParseDuration(rate)
			if err != nil {
				return fmt.Errorf("invalid rate format: %v", err)
			}

			queries, dbConn, err := getDB()
			if err != nil {
				return err
			}
			defer dbConn.Close()
			return runIngest(queries, dbConn, rateDuration)
		},
	}
	ingestCmd.Flags().StringVar(&rate, "rate", "30s", "Rate at which to generate events (e.g. 30s, 1m)")

	var pollInterval string
	var convoyAPIKey string
	var convoyProjectID string
	var convoyBaseURL string

	var workerCmd = &cobra.Command{
		Use:   "worker",
		Short: "Run in worker mode to process events",
		RunE: func(cmd *cobra.Command, args []string) error {
			pollIntervalDuration, err := time.ParseDuration(pollInterval)
			if err != nil {
				return fmt.Errorf("invalid poll interval format: %v", err)
			}

			// Initialize Convoy client with custom HTTP client
			convoyClient := convoy.New(
				convoyBaseURL,
				convoyAPIKey,
				convoyProjectID,
			)

			queries, dbConn, err := getDB()
			if err != nil {
				return err
			}
			defer dbConn.Close()
			return runWorker(queries, dbConn, pollIntervalDuration, convoyClient)
		},
	}

	workerCmd.Flags().StringVar(&pollInterval, "poll-interval", "5s", "Interval at which to poll for events (e.g. 5s, 1m)")
	workerCmd.Flags().StringVar(&convoyAPIKey, "convoy-api-key", "", "Convoy API key")
	workerCmd.Flags().StringVar(&convoyProjectID, "convoy-project-id", "", "Convoy project ID")
	workerCmd.Flags().StringVar(&convoyBaseURL, "convoy-base-url", "https://api.getconvoy.io", "Convoy API base URL")
	workerCmd.MarkFlagRequired("convoy-api-key")
	workerCmd.MarkFlagRequired("convoy-project-id")

	rootCmd.AddCommand(ingestCmd, workerCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
