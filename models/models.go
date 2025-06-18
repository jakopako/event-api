package models

import "time"

type Event struct {
	Title      string    `bson:"title,omitempty" json:"title,omitempty" validate:"required" example:"ExcitingTitle"`
	Location   string    `bson:"location,omitempty" json:"location,omitempty" validate:"required" example:"SuperLocation"`
	City       string    `bson:"city,omitempty" json:"city,omitempty" validate:"required" example:"SuperCity"`
	Country    string    `bson:"country,omitempty" json:"country,omitempty" example:"SuperCountry"`
	Date       time.Time `bson:"date,omitempty" json:"date,omitempty" validate:"required" example:"2021-10-31T19:00:00.000Z"`
	Offset     int       `bson:"offset,omitempty" json:"offset,omitempty"`
	URL        string    `bson:"url,omitempty" json:"url,omitempty" validate:"required,url" example:"http://link.to/concert/page"`
	ImageURL   string    `bson:"imageUrl,omitempty" json:"imageUrl,omitempty" validate:"omitempty,url" example:"http://link.to/concert/image.jpg"`
	Comment    string    `bson:"comment,omitempty" json:"comment,omitempty" example:"Super exciting comment."`
	Type       string    `bson:"type,omitempty" json:"type,omitempty" validate:"required" example:"concert"`
	SourceURL  string    `bson:"sourceUrl,omitempty" json:"sourceUrl,omitempty" validate:"required,url" example:"http://link.to/source"`
	Genres     []string  `bson:"genres" json:"genres" example:"german trap"`
	GenresText string    `bson:"-" json:"genresText,omitempty" example:"begleitet von diversen Berner Hip-Hop Acts. Von Trap und Phonk bis zu Afrobeats - Free Quenzy's Produktionen bieten eine breite Palette an Sounds."`
	Address    Address   `bson:"address,omitempty" json:"address"`
}

type TitleGenre struct {
	Title  string   `bson:"title"`
	Genres []string `bson:"genres"`
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

type Venue struct {
	Name    string  `bson:"name"`
	Type    string  `bson:"type"`
	Address Address `bson:"address"`
}

type Address struct {
	Locality      string           `bson:"locality" json:"locality,omitempty"`
	PostCode      string           `bson:"postCode" json:"postCode,omitempty"`
	Street        string           `bson:"street" json:"street,omitempty"`
	HouseNumber   string           `bson:"houseNumber" json:"houseNumber,omitempty"`
	Country       string           `bson:"country" json:"country,omitempty"`
	State         string           `bson:"state" json:"state,omitempty"`
	Geolocacation MongoGeolocation `bson:"geolocation" json:"geolocation"`
}

type NominatimPlace struct {
	Lat         string           `json:"lat"`
	Lon         string           `json:"lon"`
	Name        string           `json:"name"`
	DisplayName string           `json:"display_name"`
	Importance  float64          `json:"importance"`
	AddressType string           `json:"addresstype"`
	Type        string           `json:"type"`
	Address     NominatimAddress `json:"address"`
}

type NominatimAddress struct {
	HouseNumber string `json:"house_number"`
	Road        string `json:"road"`
	City        string `json:"city"`
	Town        string `json:"town"`
	Village     string `json:"village"`
	State       string `json:"state"`
	Country     string `json:"country"`
	Postcode    string `json:"postcode"`
}

type Notification struct {
	Email     string    `bson:"email" json:"email"`
	Query     Query     `bson:"query" json:"query"`
	SetupDate time.Time `bson:"setupDate" json:"setupDate"`
	Token     string    `bson:"token" json:"token"`
	Active    bool      `bson:"active" json:"active"`
}

type Query struct {
	Title     string     `bson:"title" json:"title"`
	City      string     `bson:"city" json:"city"`
	Country   string     `bson:"country" json:"country"`
	Location  string     `bson:"location" json:"location"`
	Type      string     `bson:"type" json:"type"`
	StartDate *time.Time `bson:"startDate" json:"startDate"`
	EndDate   *time.Time `bson:"endDate" json:"endDate"`
	Radius    int        `bson:"radius" json:"radius"`
	Page      int        `bson:"page" json:"-"`
	Limit     int64      `bson:"limit" json:"-"`
}

type SlackRequest struct {
	Text string `json:"text" form:"text"`
}
