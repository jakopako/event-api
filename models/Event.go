package models

import "time"

// type Concert struct {
// 	Artist   string    `bson:"artist,omitempty" json:"artist,omitempty" validate:"required" example:"SuperArtist"`
// 	Location string    `bson:"location,omitempty" json:"location,omitempty" validate:"required" example:"SuperLocation"`
// 	Date     time.Time `bson:"date,omitempty" json:"date,omitempty" validate:"required" example:"2021-10-31T19:00:00.000Z"`
// 	Link     string    `bson:"link,omitempty" json:"link,omitempty" validate:"required,url" example:"http://link.to/concert/page"`
// 	Comment  string    `bson:"comment,omitempty" json:"comment,omitempty" example:"Super exciting comment."`
// }

type EventType string

type Event struct {
	Title     string    `bson:"title,omitempty" json:"title,omitempty" validate:"required" example:"ExcitingTitle"`
	Location  string    `bson:"location,omitempty" json:"location,omitempty" validate:"required" example:"SuperLocation"`
	City      string    `bson:"city,omitempty" json:"city,omitempty" validate:"required" example:"SuperCity"`
	Date      time.Time `bson:"date,omitempty" json:"date,omitempty" validate:"required" example:"2021-10-31T19:00:00.000Z"`
	URL       string    `bson:"url,omitempty" json:"url,omitempty" validate:"required,url" example:"http://link.to/concert/page"`
	Comment   string    `bson:"comment,omitempty" json:"comment,omitempty" example:"Super exciting comment."`
	Type      EventType `bson:"type,omitempty" json:"type,omitempty" validate:"required" example:"concert"`
	SourceURL string    `bson:"sourceUrl,omitempty" json:"sourceUrl,omitempty" validate:"required" example:"http://link.to/source"`
}
