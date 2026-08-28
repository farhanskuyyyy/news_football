package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/farhanarfianto/apigo-docker/config"
	"github.com/farhanarfianto/apigo-docker/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ConnectPostgres(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName,
	)

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             2 * time.Second, // Only log queries taking > 2s to avoid spam on bulk inserts
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                  newLogger,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		// News
		&models.News{},
		// Core entities
		&models.Continent{},
		&models.Country{},
		&models.Region{},
		&models.City{},
		&models.Type{},
		// Football main entities
		&models.League{},
		&models.Season{},
		&models.Stage{},
		&models.Round{},
		&models.Team{},
		&models.Player{},
		&models.Squad{},
		&models.Fixture{},
		&models.Venue{},
		&models.Coach{},
		&models.Referee{},
		&models.Standing{},
		&models.StandingDetail{},
		&models.StandingForm{},
		&models.Topscorer{},
		&models.Transfer{},
		// Fixture sub-entities
		&models.FixtureEvent{},
		&models.FixtureLineup{},
		&models.FixtureLineupDetail{},
		&models.FixtureStatistic{},
		&models.FixtureScore{},
		&models.Commentary{},
		&models.State{},
		// Pivot / Join tables
		&models.SeasonTeam{},
		&models.PlayerSeason{},
		&models.PlayerStatistic{},
		&models.TeamRival{},
		&models.TeamCoach{},
		&models.FixtureReferee{},
		&models.SyncTable{},
	); err != nil {
		return nil, err
	}

	// Auto-seed default sync_tables configuration
	_ = SeedSyncTables(db)

	return db, nil
}

func ConnectRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
}
