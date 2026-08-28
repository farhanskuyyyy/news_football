package models

import (
	"time"
)

// League matches Sportmonks /v3/football/leagues.
type League struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID      uint      `json:"sport_id"`
	CountryID    *uint     `json:"country_id"`
	Name         string    `json:"name"`
	Active       bool      `json:"active"`
	Status       bool      `json:"status" gorm:"default:true"`
	ShortCode    string    `json:"short_code"`
	ImagePath    string    `json:"image_path"`
	Type         string    `json:"type"`
	SubType      string    `json:"sub_type"`
	LastPlayedAt *string   `json:"last_played_at"`
	Category     int       `json:"category"`
	HasJerseys   bool      `json:"has_jerseys"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// One-to-Many
	Seasons   []Season   `json:"seasons,omitempty" gorm:"foreignKey:LeagueID"`
	Stages    []Stage    `json:"stages,omitempty" gorm:"foreignKey:LeagueID"`
	Rounds    []Round    `json:"rounds,omitempty" gorm:"foreignKey:LeagueID"`
	Fixtures  []Fixture  `json:"fixtures,omitempty" gorm:"foreignKey:LeagueID"`
	Standings []Standing `json:"standings,omitempty" gorm:"foreignKey:LeagueID"`
}

// Season matches Sportmonks /v3/football/seasons.
type Season struct {
	ID                      uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID                 uint      `json:"sport_id"`
	LeagueID                uint      `json:"league_id" gorm:"index"`
	TieBreakerRuleID        *uint     `json:"tie_breaker_rule_id"`
	Name                    string    `json:"name"`
	Finished                bool      `json:"finished"`
	Pending                 bool      `json:"pending"`
	IsCurrent               bool      `json:"is_current"`
	StartingAt              *string   `json:"starting_at"`
	EndingAt                *string   `json:"ending_at"`
	StandingsRecalculatedAt *string   `json:"standings_recalculated_at"`
	GamesInCurrentWeek      bool      `json:"games_in_current_week"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`

	// Many-to-Many
	Teams   []Team   `json:"teams,omitempty" gorm:"many2many:season_teams;"`
	Players []Player `json:"players,omitempty" gorm:"many2many:player_seasons;"`
	// One-to-Many
	Stages   []Stage   `json:"stages,omitempty" gorm:"foreignKey:SeasonID"`
	Rounds   []Round   `json:"rounds,omitempty" gorm:"foreignKey:SeasonID"`
	Fixtures []Fixture `json:"fixtures,omitempty" gorm:"foreignKey:SeasonID"`
}

// Stage matches Sportmonks /v3/football/stages.
type Stage struct {
	ID                 uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID            uint      `json:"sport_id"`
	LeagueID           uint      `json:"league_id" gorm:"index"`
	SeasonID           uint      `json:"season_id" gorm:"index"`
	TypeID             *uint     `json:"type_id"`
	Name               string    `json:"name"`
	SortOrder          int       `json:"sort_order"`
	Finished           bool      `json:"finished"`
	IsCurrent          bool      `json:"is_current"`
	StartingAt         *string   `json:"starting_at"`
	EndingAt           *string   `json:"ending_at"`
	GamesInCurrentWeek bool      `json:"games_in_current_week"`
	TieBreakerRuleID   *uint     `json:"tie_breaker_rule_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Round matches Sportmonks /v3/football/rounds.
type Round struct {
	ID                 uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID            uint      `json:"sport_id"`
	LeagueID           uint      `json:"league_id" gorm:"index"`
	SeasonID           uint      `json:"season_id" gorm:"index"`
	StageID            uint      `json:"stage_id" gorm:"index"`
	Name               string    `json:"name"`
	Finished           bool      `json:"finished"`
	IsCurrent          bool      `json:"is_current"`
	StartingAt         *string   `json:"starting_at"`
	EndingAt           *string   `json:"ending_at"`
	GamesInCurrentWeek bool      `json:"games_in_current_week"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Team matches Sportmonks /v3/football/teams.
type Team struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID      uint      `json:"sport_id"`
	CountryID    *uint     `json:"country_id"`
	VenueID      *uint     `json:"venue_id"`
	Gender       string    `json:"gender"`
	Name         string    `json:"name"`
	ShortCode    string    `json:"short_code"`
	ImagePath    string    `json:"image_path"`
	Founded      *int      `json:"founded"`
	Type         string    `json:"type"`
	Placeholder  bool      `json:"placeholder"`
	LastPlayedAt *string   `json:"last_played_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Many-to-Many
	Seasons []Season `json:"seasons,omitempty" gorm:"many2many:season_teams;"`
	Players []Player `json:"players,omitempty" gorm:"many2many:squads;joinForeignKey:TeamID;joinReferences:PlayerID;"`
	Rivals  []Team   `json:"rivals,omitempty" gorm:"many2many:team_rivals;foreignKey:ID;joinForeignKey:TeamID;references:ID;joinReferences:RivalID;"`
}

// Player matches Sportmonks /v3/football/players.
type Player struct {
	ID                 uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID            uint      `json:"sport_id"`
	CountryID          *uint     `json:"country_id"`
	NationalityID      *uint     `json:"nationality_id"`
	CityID             *uint     `json:"city_id"`
	PositionID         *uint     `json:"position_id"`
	DetailedPositionID *uint     `json:"detailed_position_id"`
	TypeID             *uint     `json:"type_id"`
	CommonName         string    `json:"common_name"`
	Firstname          string    `json:"firstname"`
	Lastname           string    `json:"lastname"`
	Name               string    `json:"name"`
	DisplayName        string    `json:"display_name"`
	ImagePath          string    `json:"image_path"`
	Height             *int      `json:"height"`
	Weight             *int      `json:"weight"`
	DateOfBirth        *string   `json:"date_of_birth"`
	Gender             string    `json:"gender"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Many-to-Many
	Teams   []Team   `json:"teams,omitempty" gorm:"many2many:squads;joinForeignKey:PlayerID;joinReferences:TeamID;"`
	Seasons []Season `json:"seasons,omitempty" gorm:"many2many:player_seasons;"`
}

// Squad is the explicit pivot table connecting Team, Player, and Season.
type Squad struct {
	ID                 uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	TransferID         *uint     `json:"transfer_id"`
	PlayerID           uint      `json:"player_id" gorm:"index"`
	TeamID             uint      `json:"team_id" gorm:"index"`
	SeasonID           *uint     `json:"season_id" gorm:"index"`
	PositionID         *uint     `json:"position_id"`
	DetailedPositionID *uint     `json:"detailed_position_id"`
	Start              *string   `json:"start"`
	End                *string   `json:"end"`
	Captain            bool      `json:"captain"`
	JerseyNumber       *int      `json:"jersey_number"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Fixture matches Sportmonks /v3/football/fixtures.
type Fixture struct {
	ID                  uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID             uint      `json:"sport_id"`
	LeagueID            uint      `json:"league_id" gorm:"index"`
	SeasonID            uint      `json:"season_id" gorm:"index"`
	StageID             *uint     `json:"stage_id"`
	GroupID             *uint     `json:"group_id"`
	AggregateID         *uint     `json:"aggregate_id"`
	RoundID             *uint     `json:"round_id"`
	StateID             *uint     `json:"state_id"`
	VenueID             *uint     `json:"venue_id"`
	Name                string    `json:"name"`
	StartingAt          *string   `json:"starting_at"`
	ResultInfo          *string   `json:"result_info"`
	Leg                 string    `json:"leg"`
	Details             *string   `json:"details"`
	Length              int       `json:"length"`
	Placeholder         bool      `json:"placeholder"`
	HasOdds             bool      `json:"has_odds"`
	StartingAtTimestamp *int64    `json:"starting_at_timestamp"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// One-to-Many (Fixture sub-entities stored in separate tables)
	Events     []FixtureEvent     `json:"events,omitempty" gorm:"foreignKey:FixtureID"`
	Lineups    []FixtureLineup    `json:"lineups,omitempty" gorm:"foreignKey:FixtureID"`
	Statistics []FixtureStatistic `json:"statistics,omitempty" gorm:"foreignKey:FixtureID"`
	Scores     []FixtureScore     `json:"scores,omitempty" gorm:"foreignKey:FixtureID"`
	// Belongs-To
	State *State `json:"state,omitempty" gorm:"foreignKey:StateID;constraint:false;"`
	// Many-to-Many
	Referees []Referee `json:"referees,omitempty" gorm:"many2many:fixture_referees;"`
}

// Venue matches Sportmonks /v3/football/venues.
type Venue struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	CountryID    *uint     `json:"country_id"`
	CityID       *uint     `json:"city_id"`
	Name         string    `json:"name"`
	Address      *string   `json:"address"`
	Zipcode      *string   `json:"zipcode"`
	Latitude     *string   `json:"latitude"`
	Longitude    *string   `json:"longitude"`
	Capacity     *int      `json:"capacity"`
	ImagePath    string    `json:"image_path"`
	CityName     string    `json:"city_name"`
	Surface      string    `json:"surface"`
	NationalTeam bool      `json:"national_team"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Coach matches Sportmonks /v3/football/coaches.
type Coach struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	PlayerID      *uint     `json:"player_id"`
	SportID       uint      `json:"sport_id"`
	CountryID     *uint     `json:"country_id"`
	NationalityID *uint     `json:"nationality_id"`
	CityID        *uint     `json:"city_id"`
	CommonName    string    `json:"common_name"`
	Firstname     string    `json:"firstname"`
	Lastname      string    `json:"lastname"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"display_name"`
	ImagePath     string    `json:"image_path"`
	Height        *int      `json:"height"`
	Weight        *int      `json:"weight"`
	DateOfBirth   *string   `json:"date_of_birth"`
	Gender        string    `json:"gender"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Referee matches Sportmonks /v3/football/referees.
type Referee struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID     uint      `json:"sport_id"`
	CountryID   *uint     `json:"country_id"`
	CityID      *uint     `json:"city_id"`
	CommonName  string    `json:"common_name"`
	Firstname   *string   `json:"firstname"`
	Lastname    *string   `json:"lastname"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	ImagePath   string    `json:"image_path"`
	Height      *int      `json:"height"`
	Weight      *int      `json:"weight"`
	DateOfBirth *string   `json:"date_of_birth"`
	Gender      *string   `json:"gender"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Many-to-Many
	Fixtures []Fixture `json:"fixtures,omitempty" gorm:"many2many:fixture_referees;"`
}

// Standing matches Sportmonks /v3/football/standings.
type Standing struct {
	ID             uint             `json:"id" gorm:"primaryKey;autoIncrement:false"`
	ParticipantID  uint             `json:"participant_id" gorm:"index"`
	SportID        uint             `json:"sport_id"`
	LeagueID       uint             `json:"league_id" gorm:"index"`
	SeasonID       uint             `json:"season_id" gorm:"index"`
	StageID        *uint            `json:"stage_id"`
	GroupID        *uint            `json:"group_id"`
	RoundID        *uint            `json:"round_id"`
	StandingRuleID *uint            `json:"standing_rule_id"`
	Position       int              `json:"position"`
	Result         string           `json:"result"`
	Points         int              `json:"points"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	Details        []StandingDetail `json:"details,omitempty" gorm:"foreignKey:StandingID;constraint:false;"`
}

// StandingDetail matches Sportmonks standing.details sub-entity.
type StandingDetail struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	StandingID   uint      `json:"standing_id" gorm:"index"`
	StandingType string    `json:"standing_type"`
	TypeID       uint      `json:"type_id" gorm:"index"`
	Value        int       `json:"value"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Type         *Type     `json:"type,omitempty" gorm:"foreignKey:TypeID;constraint:false;"`
}

// Topscorer matches Sportmonks /v3/football/topscorers.
type Topscorer struct {
	ID              uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SeasonID        *uint     `json:"season_id" gorm:"index"`
	StageID         *uint     `json:"stage_id" gorm:"index"`
	LeagueID        *uint     `json:"league_id" gorm:"index"`
	PlayerID        uint      `json:"player_id" gorm:"index"`
	TypeID          uint      `json:"type_id"`
	Position        int       `json:"position"`
	Total           int       `json:"total"`
	ParticipantType string    `json:"participant_type"`
	ParticipantID   uint      `json:"participant_id" gorm:"index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Transfer matches Sportmonks /v3/football/transfers.
type Transfer struct {
	ID                 uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID            uint      `json:"sport_id"`
	PlayerID           uint      `json:"player_id" gorm:"index"`
	TypeID             *uint     `json:"type_id"`
	FromTeamID         *uint     `json:"from_team_id" gorm:"index"`
	ToTeamID           *uint     `json:"to_team_id" gorm:"index"`
	PositionID         *uint     `json:"position_id"`
	DetailedPositionID *uint     `json:"detailed_position_id"`
	Date               *string   `json:"date"`
	CareerEnded        bool      `json:"career_ended"`
	Completed          bool      `json:"completed"`
	Amount             *int64    `json:"amount"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// SyncTable tracks synchronization metadata, latest sync timestamp, and dynamic TTL intervals per dataset.
type SyncTable struct {
	TableName       string    `json:"table_name" gorm:"primaryKey"`
	LatestSyncedAt  time.Time `json:"latest_synced_at"`
	IntervalSeconds int       `json:"interval_seconds"`
	RecordsSynced   int       `json:"records_synced"`
	Status          string    `json:"status"` // "success", "failed", "in_progress", "skipped"
	ErrorMessage    string    `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

