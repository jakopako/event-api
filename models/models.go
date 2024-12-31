package models

import "time"

type EventType string

type Event struct {
	Title            string           `bson:"title,omitempty" json:"title,omitempty" validate:"required" example:"ExcitingTitle"`
	Location         string           `bson:"location,omitempty" json:"location,omitempty" validate:"required" example:"SuperLocation"`
	City             string           `bson:"city,omitempty" json:"city,omitempty" validate:"required" example:"SuperCity"`
	Country          string           `bson:"country,omitempty" json:"country,omitempty" example:"SuperCountry"`
	Date             time.Time        `bson:"date,omitempty" json:"date,omitempty" validate:"required" example:"2021-10-31T19:00:00.000Z"`
	Offset           int              `bson:"offset,omitempty" json:"offset,omitempty"`
	URL              string           `bson:"url,omitempty" json:"url,omitempty" validate:"required,url" example:"http://link.to/concert/page"`
	ImageURL         string           `bson:"imageUrl,omitempty" json:"imageUrl,omitempty" validate:"omitempty,url" example:"http://link.to/concert/image.jpg"`
	Comment          string           `bson:"comment,omitempty" json:"comment,omitempty" example:"Super exciting comment."`
	Type             EventType        `bson:"type,omitempty" json:"type,omitempty" validate:"required" example:"concert"`
	SourceURL        string           `bson:"sourceUrl,omitempty" json:"sourceUrl,omitempty" validate:"required,url" example:"http://link.to/source"`
	Geolocation      []float64        `bson:"-" json:"geolocation,omitempty" example:"7.4514512,46.9482713"`
	MongoGeolocation MongoGeolocation `bson:"geolocation,omitempty" json:"-"`
	Genres           []string         `bson:"genres,omitempty" json:"genres,omitempty" example:"german trap"`
}

type MongoGeolocation struct {
	GeoJSONType string    `json:"type" bson:"type,omitempty"`
	Coordinates []float64 `json:"coordinates" bson:"coordinates,omitempty"`
}

type City struct {
	Name        string           `bson:"name"`
	Country     string           `bson:"country"`
	Geolocation MongoGeolocation `bson:"geolocation"`
}

type NominatimPlace struct {
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	DisplayName string  `json:"display_name"`
	Importance  float64 `json:"importance"`
}

type Notification struct {
	Email     string    `bson:"email" json:"email"`
	Query     Query     `bson:"query" json:"query"`
	SetupDate time.Time `bson:"setupDate" json:"setupDate"`
	Token     string    `bson:"token" json:"token"`
	Active    bool      `bson:"active" json:"active"`
}

type Query struct {
	Title    string `bson:"title" json:"title"`
	City     string `bson:"city" json:"city"`
	Country  string `bson:"country" json:"country"`
	Location string `bson:"location" json:"location"`
	Date     string `bson:"date" json:"date"`
	Radius   int    `bson:"radius" json:"radius"`
	Page     int    `bson:"page" json:"-"`
	Limit    int64  `bson:"limit" json:"-"`
}
