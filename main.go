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
)

const indexName = "items"

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
	// Create context.
	ctx := context.Background()

	// Create new client.
	client, err := elastic.NewClient()
	if err != nil {
		panic(err)
	}

	// Delete an index.
	deleteIndex, err := client.DeleteIndex(indexName).Do(ctx)
	if err != nil {
		panic(err)
	}
	if !deleteIndex.Acknowledged {
		fmt.Printf("Index not acknowledged")
	}

	// Check if index already exists.
	exists, err := client.IndexExists(indexName).Do(ctx)
	if err != nil {
		panic(err)
	}
	if !exists {
		// Create a new index.
		createIndex, err := client.CreateIndex(indexName).BodyString(mapping).Do(ctx)
		if err != nil {
			panic(err)
		}
		if !createIndex.Acknowledged {
			fmt.Printf("Index not acknowledged")
		}
	}

	// Populate some items.
	items := []schema.Item{
		{Name: "pedestal", Description: "3-tier white-colored pedestal.", Stock: 1},
		{Name: "desk", Description: "Black wooden desk.", Stock: 15},
		{Name: "monitor", Description: "LG monitor complete with cable.", Stock: 2},
		{Name: "monitor", Description: "Samsung monitor complete with cable.", Stock: 2},
		{Name: "monitor", Description: "Apple monitor complete with cable.", Stock: 2},
		{Name: "monitor", Description: "Dell monitor complete with cable.", Stock: 2},
		{Name: "laptop", Description: "Macbook Pro 2017 13-inch.", Stock: 30},
		{Name: "mouse", Description: "Logitech M100 black mouse.", Stock: 4},
		{Name: "mouse pad", Description: "Plain black mouse pad.", Stock: 100},
		{Name: "mug", Description: "Mug with Tokopedia logo.", Stock: 55},
		{Name: "notebook", Description: "A4 notebook with strap.", Stock: 6},
		{Name: "shirt", Description: "Black t-shirt with Tokopedia logo.", Stock: 9},
		{Name: "green chair", Description: "Green chair from the USA.", Stock: 9},
		{Name: "black chair", Description: "Black chair from the UK.", Stock: 9},
	}
	for _, item := range items {
		_, err = client.Index().
			Index(indexName).
			Type("item").
			BodyJson(item).
			Do(ctx)
	}
	if err != nil {
		panic(err)
	}

	// Flush to make sure the documents got written.
	_, err = client.Flush().Index(indexName).Do(ctx)
	if err != nil {
		panic(err)
	}

	// Page
	welcome := schema.Welcome{"Nakama"}
	templates := template.Must(template.ParseFiles(
		"templates/landing-page.html",
		"templates/item.html",
		"templates/create.html",
		"templates/list.html",
		"templates/edit.html"))
	http.Handle("/static/", //final url can be anything
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("static"))))

	// Landing page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Set welcome message name according to URL param
		if username := r.FormValue("username"); username != "" {
			welcome.Username = username
		}
		if r.Method == "POST" {
			if name := r.FormValue("name"); name != "" {
				http.Redirect(w, r, "/search?name=" + name, http.StatusSeeOther)
			}
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
				Index(indexName).
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
	http.HandleFunc("/create/", func(w http.ResponseWriter, r *http.Request) {
		item := schema.Item {
			Name: r.FormValue("name"),
			Description: r.FormValue("description"),
		}

		// Index a item (using JSON serialization)
		newItem := schema.Item{Name: item.Name, Description: item.Description, Stock: 1}
		putItem, err := client.Index().
			Index(indexName).
			Type("item").
			BodyJson(newItem).
			Do(ctx)
		if err != nil {
			panic(err)
		}

		// Flush to make sure the documents got written.
		_, err = client.Flush().Index(indexName).Do(ctx)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Indexed item %s to index %s, type %s\n", putItem.Id, putItem.Index, putItem.Type)

		if err := templates.ExecuteTemplate(w, "create.html", item); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Edit item page
	http.HandleFunc("/edit/", func(w http.ResponseWriter, r *http.Request) {
		// Get item
		var item schema.Item
		if id := r.FormValue("id"); id != "" {
			// Get item with specified ID
			itemResult, err := client.Get().
				Index(indexName).
				Type("item").
				Id(id).
				Do(ctx)
			if err != nil {
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
		if r.Method == "POST" {
			if id := r.FormValue("id"); id != "" {
				update, err := client.Update().Index(indexName).Type("item").Id(id).
					Script(elastic.NewScriptInline("ctx._source.name = params.name").Lang("painless").Param("name", item.Name)).
					Upsert(map[string]interface{}{"name": ""}).
					Do(ctx)
				if err != nil {
					panic(err)
				}
				fmt.Printf("New version of item %q is now %d\n", update.Id, update.Version)
				// Flush to make sure the documents got written.
				_, err = client.Flush().Index(indexName).Do(ctx)
				if err != nil {
					panic(err)
				}
				// Flush to make sure the documents got written.
				_, err = client.Flush().Index(indexName).Do(ctx)
				if err != nil {
					panic(err)
				}

				http.Redirect(w, r, "/items?id=" + id, http.StatusSeeOther)
			}
		}

		if err := templates.ExecuteTemplate(w, "edit.html", item); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Search item.
	http.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) {
		var items []schema.Item
		if name := r.FormValue("name"); name != "" {
			termQuery := elastic.NewTermQuery("name", name)
			searchResult, err := client.Search().
				Index(indexName).
				Query(termQuery).
				Sort("name", true).
				From(0).Size(100).
				Pretty(true).
				Do(ctx)
			if err != nil {
				panic(err)
			}

			var ttyp schema.Item
			for _, item := range searchResult.Each(reflect.TypeOf(ttyp)) {
				if t, ok := item.(schema.Item); ok {
					fmt.Printf("Item named %s: %s\n", t.Name, t.Description)
				}
			}

			if searchResult.Hits.TotalHits > 0 {
				for _, hit := range searchResult.Hits.Hits {
					var t schema.Item
					err := json.Unmarshal(*hit.Source, &t)
					if err != nil {
						// Deserialization failed
					}

					// Work with item
					fmt.Printf("Item named %s: %s\n", t.Name, t.Description)
					items = append(items, t)
				}
			} else {
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
