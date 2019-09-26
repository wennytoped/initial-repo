package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/olivere/elastic.v6"
	//"html/template"
	//"net/http"
	"reflect"
	"time"
)

// Welcome message
type Welcome struct {
	Name string
	Time string
}

// Item inside the inventory
//type Item struct {
//	Id int64 `json:"id"`
//	Name string `json:"name"`
//	Location string `json:"location,omitempty"`
//	Stock int64 `json:"stock,omitempty"`
//	Image string `json:"image,omitempty"`
//}

// Item is a structure used for serializing/deserializing data in Elasticsearch.
type Item struct {
	Name     string                `json:"name"`
	Description  string                `json:"description"`
	Stock int                   `json:"stock"`
	Image    string                `json:"image,omitempty"`
	Created  time.Time             `json:"created,omitempty"`
	Tags     []string              `json:"tags,omitempty"`
	Location string                `json:"location,omitempty"`
	Suggest  *elastic.SuggestField `json:"suggest_field,omitempty"`
}

const mapping = `
{
	"settings":{
		"number_of_shards": 1,
		"number_of_replicas": 0
	},
	"mappings":{
		"item":{
			"properties":{
				"name":{
					"type":"keyword"
				},
				"description":{
					"type":"text",
					"store": true,
					"fielddata": true
				},
				"image":{
					"type":"keyword"
				},
				"created":{
					"type":"date"
				},
				"tags":{
					"type":"keyword"
				},
				"location":{
					"type":"geo_point"
				},
				"suggest_field":{
					"type":"completion"
				}
			}
		}
	}
}`

func main() {
	// Starting with elastic.v5, you must pass a context to execute each service
	ctx := context.Background()

	// Obtain a client and connect to the default Elasticsearch installation
	// on 127.0.0.1:9200. Of course you can configure your client to connect
	// to other hosts and configure it in various other ways.
	client, err := elastic.NewClient()
	if err != nil {
		// Handle error
		panic(err)
	}

	// Ping the Elasticsearch server to get e.g. the version number
	info, code, err := client.Ping("http://127.0.0.1:9200").Do(ctx)
	if err != nil {
		// Handle error
		panic(err)
	}
	fmt.Printf("Elasticsearch returned with code %d and version %s\n", code, info.Version.Number)

	// Getting the ES version number is quite common, so there's a shortcut
	esversion, err := client.ElasticsearchVersion("http://127.0.0.1:9200")
	if err != nil {
		// Handle error
		panic(err)
	}
	fmt.Printf("Elasticsearch version %s\n", esversion)

	// Use the IndexExists service to check if a specified index exists.
	exists, err := client.IndexExists("inventopedia").Do(ctx)
	if err != nil {
		// Handle error
		panic(err)
	}
	if !exists {
		// Create a new index.
		createIndex, err := client.CreateIndex("inventopedia").BodyString(mapping).Do(ctx)
		if err != nil {
			// Handle error
			panic(err)
		}
		if !createIndex.Acknowledged {
			// Not acknowledged
		}
	}

	// Index a item (using JSON serialization)
	item1 := Item{Name: "Chair", Description: "A green chair imported from the USA.", Stock: 0}
	put1, err := client.Index().
		Index("inventopedia").
		Type("item").
		Id("1").
		BodyJson(item1).
		Do(ctx)
	if err != nil {
		// Handle error
		panic(err)
	}
	fmt.Printf("Indexed item %s to index %s, type %s\n", put1.Id, put1.Index, put1.Type)

	// Index a second item (by string)
	item2 := `{"name" : "Laptop", "description" : "Macbook Pro 15-inch"}`
	put2, err := client.Index().
		Index("inventopedia").
		Type("item").
		Id("2").
		BodyString(item2).
		Do(ctx)
	if err != nil {
		// Handle error
		panic(err)
	}
	fmt.Printf("Indexed item %s to index %s, type %s\n", put2.Id, put2.Index, put2.Type)

	// Get item with specified ID
	get1, err := client.Get().
		Index("inventopedia").
		Type("item").
		Id("1").
		Do(ctx)
	if err != nil {
		// Handle error
		panic(err)
	}
	if get1.Found {
		fmt.Printf("Got document %s in version %d from index %s, type %s\n", get1.Id, get1.Version, get1.Index, get1.Type)
	}

	// Flush to make sure the documents got written.
	_, err = client.Flush().Index("inventopedia").Do(ctx)
	if err != nil {
		panic(err)
	}

	// Search with a term query
	termQuery := elastic.NewTermQuery("name", "Laptop")
	searchResult, err := client.Search().
		Index("inventopedia").   // search in index "inventopedia"
		Query(termQuery).   // specify the query
		Sort("name", true). // sort by "name" field, ascending
		From(0).Size(10).   // take documents 0-9
		Pretty(true).       // pretty print request and response JSON
		Do(ctx)             // execute
	if err != nil {
		// Handle error
		panic(err)
	}

	// searchResult is of type SearchResult and returns hits, suggestions,
	// and all kinds of other information from Elasticsearch.
	fmt.Printf("Query took %d milliseconds\n", searchResult.TookInMillis)

	// Each is a convenience function that iterates over hits in a search result.
	// It makes sure you don't need to check for nil values in the response.
	// However, it ignores errors in serialization. If you want full control
	// over iterating the hits, see below.
	var ttyp Item
	for _, item := range searchResult.Each(reflect.TypeOf(ttyp)) {
		if t, ok := item.(Item); ok {
			fmt.Printf("Item named %s: %s\n", t.Name, t.Description)
		}
	}

	// Here's how you iterate through results with full control over each step.
	if searchResult.Hits.TotalHits > 0 {
		fmt.Printf("Found a total of %d items\n", searchResult.Hits.TotalHits)

		// Iterate through results
		for _, hit := range searchResult.Hits.Hits {
			// hit.Index contains the name of the index

			// Deserialize hit.Source into a Item (could also be just a map[string]interface{}).
			var t Item
			err := json.Unmarshal(*hit.Source, &t)
			if err != nil {
				// Deserialization failed
			}

			// Work with item
			fmt.Printf("Item by %s: %s\n", t.Name, t.Description)
		}
	} else {
		// No hits
		fmt.Print("Found no items\n")
	}

	// Update a item by the update API of Elasticsearch.
	// We just increment the number of stock.
	update, err := client.Update().Index("inventopedia").Type("item").Id("1").
		Script(elastic.NewScriptInline("ctx._source.stock += params.num").Lang("painless").Param("num", 1)).
		Upsert(map[string]interface{}{"stock": 0}).
		Do(ctx)
	if err != nil {
		// Handle error
		panic(err)
	}
	fmt.Printf("New version of item %q is now %d\n", update.Id, update.Version)

	// ...

	// Delete an index.
	deleteIndex, err := client.DeleteIndex("inventopedia").Do(ctx)
	if err != nil {
		// Handle error
		panic(err)
	}
	if !deleteIndex.Acknowledged {
		// Not acknowledged
	}

	//	welcome := Welcome{"Anonymous", time.Now().Format(time.Stamp)}
	//	templates := template.Must(template.ParseFiles("templates/landing-page.html"))
	//	http.Handle("/static/", //final url can be anything
	//		http.StripPrefix("/static/",
	//			http.FileServer(http.Dir("static"))))
	//
	//	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	//		// Set welcome message name according to URL param
	//		if name := r.FormValue("name"); name != "" {
	//			welcome.Name = name
	//		}
	//
	//		if err := templates.ExecuteTemplate(w, "landing-page.html", welcome); err != nil {
	//			http.Error(w, err.Error(), http.StatusInternalServerError)
	//		}
	//	})
	//
	//	fmt.Println("Listening on port :8080")
	//	fmt.Println(http.ListenAndServe(":8080", nil))
}
