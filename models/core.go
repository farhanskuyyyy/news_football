package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// StringArray is a custom type to serialize/deserialize string slices for DB storage.
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	return json.Marshal(a)
}

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = StringArray{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return nil
	}
	return json.Unmarshal(bytes, a)
}

// Continent matches Sportmonks /v3/core/continents response.
type Continent struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Country matches Sportmonks /v3/core/countries response.
type Country struct {
	ID           uint        `json:"id" gorm:"primaryKey;autoIncrement:false"`
	ContinentID  *uint       `json:"continent_id"`
	Name         string      `json:"name"`
	OfficialName string      `json:"official_name"`
	FifaName     string      `json:"fifa_name"`
	Iso2         string      `json:"iso2"`
	Iso3         string      `json:"iso3"`
	Latitude     string      `json:"latitude"`
	Longitude    string      `json:"longitude"`
	Borders      StringArray `json:"borders" gorm:"type:text"`
	ImagePath    string      `json:"image_path"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// Region matches Sportmonks /v3/core/regions response.
type Region struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	CountryID uint      `json:"country_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// City matches Sportmonks /v3/core/cities response.
type City struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	CountryID uint      `json:"country_id"`
	RegionID  *uint     `json:"region_id"`
	Name      string    `json:"name"`
	Latitude  string    `json:"latitude"`
	Longitude string    `json:"longitude"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Type matches Sportmonks /v3/core/types response.
type Type struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Name          string    `json:"name"`
	Code          string    `json:"code"`
	DeveloperName string    `json:"developer_name"`
	ModelType     string    `json:"model_type"`
	StatGroup     *string   `json:"stat_group"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
