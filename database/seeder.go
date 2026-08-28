package database

import (
	"log"
	"time"

	"github.com/farhanarfianto/apigo-docker/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultSyncTableConfigs provides default intervals for each dataset/table.
var DefaultSyncTableConfigs = []models.SyncTable{
	// 1 Hour (Live & Frequent Updates)
	{TableName: "fixtures", IntervalSeconds: 3600, Status: "pending"},
	{TableName: "standings", IntervalSeconds: 3600, Status: "pending"},
	{TableName: "topscorers", IntervalSeconds: 3600, Status: "pending"},

	// 12 Hours (Semi-frequent)
	{TableName: "transfers", IntervalSeconds: 43200, Status: "pending"},

	// 24 Hours (Daily)
	{TableName: "squads", IntervalSeconds: 86400, Status: "pending"},
	{TableName: "players", IntervalSeconds: 86400, Status: "pending"},
	{TableName: "rounds", IntervalSeconds: 86400, Status: "pending"},
	{TableName: "stages", IntervalSeconds: 86400, Status: "pending"},

	// 7 Days (Weekly)
	{TableName: "teams", IntervalSeconds: 604800, Status: "pending"},
	{TableName: "seasons", IntervalSeconds: 604800, Status: "pending"},

	// 30 Days (Monthly / Static Reference Data)
	{TableName: "leagues", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "venues", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "coaches", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "referees", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "rivals", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "states", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "continents", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "countries", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "regions", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "cities", IntervalSeconds: 2592000, Status: "pending"},
	{TableName: "types", IntervalSeconds: 2592000, Status: "pending"},
}

// SeedSyncTables populates default sync configuration for tables if not already present.
func SeedSyncTables(db *gorm.DB) error {
	now := time.Now()
	var records []models.SyncTable
	for _, cfg := range DefaultSyncTableConfigs {
		item := cfg
		item.CreatedAt = now
		item.UpdatedAt = now
		records = append(records, item)
	}

	// Insert only if record doesn't exist (DoNothing on conflict)
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "table_name"}},
		DoNothing: true,
	}).Create(&records).Error

	if err != nil {
		log.Printf("[Seeder] Error seeding sync_tables: %v", err)
		return err
	}

	log.Printf("[Seeder] Successfully seeded %d sync_table configurations", len(records))
	return nil
}
