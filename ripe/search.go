package ripe

import (
	"encoding/json"
	"strings"
)

type Range struct {
	Name  string
	Range string
	Route string
}

func (c *Controller) Search(search string) (ranges []Range, err error) {
	// Prepare URL
	searchSplitted := strings.Split(search, " ")
	url := *baseURL
	queryValues := url.Query()
	queryValues.Set("q", "("+strings.Join(searchSplitted, " AND ")+")")
	url.RawQuery = queryValues.Encode()
	// Start request
	resp, err := c.client.Get(url.String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	// Decode
	var payload result
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&payload)
	// Extract results
	return extractRange(payload)
}
