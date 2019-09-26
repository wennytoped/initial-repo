package schema

import (
	"gopkg.in/olivere/elastic.v6"
	"time"
)

// Welcome message
type Welcome struct {
	Username string
}

// Item is a structure used for serializing/deserializing data in Elasticsearch.
type Item struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Stock       int                   `json:"stock"`
	Image       string                `json:"image,omitempty"`
	Created     time.Time             `json:"created,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Location    string                `json:"location,omitempty"`
	Suggest     *elastic.SuggestField `json:"suggest_field,omitempty"`
}

// Response for search page
type SearchResponse struct {
	Item    []Item `json:"item"`
	Message string `json:"string"`
}
