# fastentity
[![Build Status](https://travis-ci.org/sajari/fastentity.svg?branch=master)](https://travis-ci.org/sajari/fastentity)

Fast discovery of huge lists of character sequences, e.g. text sequence identification in documents using Go.

## Introduction

### What this does do:
Fast, multi-lingual character sequence detection in plain text. This enables you to specify many known entities, such as lists of names, places, things, etc, and quickly detect them in sequences of text or documents. It is case insensitive, but retains the case of the detected text, so it can be further evaluated once detected.

Currently this is used to look for names, skills, etc. A database of ~500,000 entities can routinely be detected from ~2000 chars (~1 page) of text in ~15msec (single threaded). This is closer to 1msec for 20,000 entities, so depending on your application, this is quite fast and scales well with larger entity sets. 

Entities can be added programatically or by CSV.


### What this doesn't do:
Currently it does not look at language structure. It is purely looking for known sequences.

It does not currently handle streams of text.


## Getting Started
### Installing

To start using fastentity, install Go and run `go get`:

```sh
$ go get github.com/sajari/fastentity
```

### Sample usage
```go
package main

import (
	"fmt"
	"github.com/sajari/fastentity"
)

func main() {
	str := []rune("日 本語. Jack was a Golang developer from sydney. San Francisco, USA... Or so they say.")

	// Create a store
	store := fastentity.New("locations", "jobTitles")

	// Add single entities
	store.Add("locations", []rune("San Francisco, USA"))
	store.Add("jobTitles", []rune("golang developer"))

	// Add multiple (note: You don't need to initialise each group, they will be auto created if they don't exist)
	store.Add("skills", []rune("本語"), []rune("golang")) 

	results := store.FindAll(str)

	for group, entities := range results {
		fmt.Printf("Group: %s \n", group)
		for _, entity := range entities {
			fmt.Printf("\t-> %s\n", string(entity))
		}
	}
	/* Prints
	Group: locations
		-> San Francisco, USA
	Group: jobTitles
		-> golang developer
	Group: skills
		-> 本語
	*/
}
```

## Future changes
- Look at surrounding structure as part of identification
- Allow functions to be passed with each group detection, e.g. boolean check if first letter is a capital, etc
- Apply to streams of text
- Concurrency

## Saving and loading data
You can save and load entity groups in CSV files. Each group is saved to it's own file. To load an entity set, point the store to the directory containing the entity files. 
```go
if store, err = fastentity.FromDir("path_to_load_csv_files"); err != nil {
	fmt.Printf("Failed to load store: %v", err)
}
```

Same to save them
```go
err:= store.Save("path_to_save_csv_files")
```
