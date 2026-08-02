package models

import "time"

type News struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Source      string    `json:"source"`
	Author      string    `json:"author"`
	Title       string    `json:"title"`
	Description string    `json:"description" gorm:"type:text"`
	URL         string    `json:"url" gorm:"uniqueIndex;size:512"`
	URLToImage  string    `json:"url_to_image"`
	PublishedAt time.Time `json:"published_at"`
	Content     string    `json:"content" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
