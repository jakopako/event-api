package models

import "time"

type EventType string

type Event struct {
	Title            string           `bson:"title,omitempty" json:"title,omitempty" validate:"required" example:"ExcitingTitle"`
	Location         string           `bson:"location,omitempty" json:"location,omitempty" validate:"required" example:"SuperLocation"`
	City             string           `bson:"city,omitempty" json:"city,omitempty" validate:"required" example:"SuperCity"`
	Country          string           `bson:"country,omitempty" json:"country,omitempty" example:"SuperCountry"`
	Date             time.Time        `bson:"date,omitempty" json:"date,omitempty" validate:"required" example:"2021-10-31T19:00:00.000Z"`
	URL              string           `bson:"url,omitempty" json:"url,omitempty" validate:"required,url" example:"http://link.to/concert/page"`
	Comment          string           `bson:"comment,omitempty" json:"comment,omitempty" example:"Super exciting comment."`
	Type             EventType        `bson:"type,omitempty" json:"type,omitempty" validate:"required" example:"concert"`
	SourceURL        string           `bson:"sourceUrl,omitempty" json:"sourceUrl,omitempty" validate:"required" example:"http://link.to/source"`
	Geolocation      []float64        `bson:"-" json:"geolocation,omitempty" example:"7.4514512,46.9482713"`
	MongoGeolocation MongoGeolocation `bson:"geolocation,omitempty" json:"-"`
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
