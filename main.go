package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/olivere/elastic.v6"
	"html/template"
	"invento-search/schema"
	"net/http"
	"reflect"
	"time"
)

const mapping = `
{
	"settings":{
		"number_of_shards": 1,
		"number_of_replicas": 0
	},
	"mappings":{
		"item":{
			"properties":{
				"id": {
					"type":"text"
				},
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
	ctx := context.Background()

	client, err := elastic.NewClient()
	if err != nil {
		panic(err)
	}

	// Getting the ES version number is quite common, so there's a shortcut
	esversion, err := client.ElasticsearchVersion("http://127.0.0.1:9200")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Elasticsearch version %s\n", esversion)

	// Check if inventopedia index exists.
	exists, err := client.IndexExists("inventopedia").Do(ctx)
	if err != nil {
		panic(err)
	}
	if !exists {
		// Create a new index.
		createIndex, err := client.CreateIndex("inventopedia").BodyString(mapping).Do(ctx)
		if err != nil {
			panic(err)
		}
		if !createIndex.Acknowledged {
			fmt.Printf("Index not acknowledged")
		}
	}

	// Index a item (using JSON serialization)
	item1 := schema.Item{Name: "Chair", Description: "A green chair imported from the USA.", Stock: 0}
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

	// Flush to make sure the documents got written.
	_, err = client.Flush().Index("inventopedia").Do(ctx)
	if err != nil {
		panic(err)
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

	//// Delete an index.
	//deleteIndex, err := client.DeleteIndex("inventopedia").Do(ctx)
	//if err != nil {
	//	// Handle error
	//	panic(err)
	//}
	//if !deleteIndex.Acknowledged {
	//	// Not acknowledged
	//}

	// Page
	welcome := schema.Welcome{"Nakama", time.Now().Format(time.Stamp)}
	templates := template.Must(template.ParseFiles("templates/landing-page.html", "templates/item.html", "templates/create.html", "templates/list.html"))
	http.Handle("/static/", //final url can be anything
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("static"))))

	// Landing page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Set welcome message name according to URL param
		if name := r.FormValue("name"); name != "" {
			welcome.Name = name
		}

		if err := templates.ExecuteTemplate(w, "landing-page.html", welcome); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Item page
	http.HandleFunc("/items/", func(w http.ResponseWriter, r *http.Request) {
		// Set welcome message name according to URL param
		var item schema.Item

		if id := r.FormValue("id"); id != "" {
			// Get item with specified ID
			itemResult, err := client.Get().
				Index("inventopedia").
				Type("item").
				Id(id).
				Do(ctx)
			if err != nil {
				// Handle error
				panic(err)
			}
			if itemResult.Found {
				fmt.Printf("Got document %s in version %d from index %s, type %s\n", itemResult.Id, itemResult.Version, itemResult.Index, itemResult.Type)
				err := json.Unmarshal(*itemResult.Source, &item)
				if err != nil {
					panic(err)
				}
			} else {
				fmt.Printf("Document %s not found", id)
			}
		}

		if err := templates.ExecuteTemplate(w, "item.html", item); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Create item page
	// Landing page
	http.HandleFunc("/create/", func(w http.ResponseWriter, r *http.Request) {
		item := schema.Item{
			Name:        r.FormValue("name"),
			Description: r.FormValue("description"),
		}

		// Index a item (using JSON serialization)
		newItem := schema.Item{Name: item.Name, Description: item.Description, Stock: 1}
		putItem, err := client.Index().
			Index("inventopedia").
			Type("item").
			BodyJson(newItem).
			Do(ctx)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Indexed item %s to index %s, type %s\n", putItem.Id, putItem.Index, putItem.Type)

		if err := templates.ExecuteTemplate(w, "create.html", item); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) {
		var items []schema.Item
		if name := r.FormValue("name"); name != "" {
			// Search with a term query
			termQuery := elastic.NewTermQuery("name", name)
			searchResult, err := client.Search().
				Index("inventopedia"). // search in index "inventopedia"
				Query(termQuery). // specify the query
				Sort("name", true). // sort by "name" field, ascending
				From(0).Size(10). // take documents 0-9
				Pretty(true). // pretty print request and response JSON
				Do(ctx) // execute
			if err != nil {
				panic(err)
			}

			var ttyp schema.Item
			for _, item := range searchResult.Each(reflect.TypeOf(ttyp)) {
				if t, ok := item.(schema.Item); ok {
					fmt.Printf("Item named %s: %s\n", t.Name, t.Description)
				}
			}

			// Here's how you iterate through results with full control over each step.
			if searchResult.Hits.TotalHits > 0 {
				for _, hit := range searchResult.Hits.Hits {
					var t schema.Item
					err := json.Unmarshal(*hit.Source, &t)
					if err != nil {
						// Deserialization failed
					}

					// Work with item
					fmt.Printf("Item by %s: %s\n", t.Name, t.Description)
					items = append(items, t)
				}
			} else {
				// No hits
				fmt.Print("Found no items\n")
			}
		}

		if err := templates.ExecuteTemplate(w, "list.html", items); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	fmt.Println("Listening on port :8080")
	fmt.Println(http.ListenAndServe(":8080", nil))
}
