package sessions

// Stats counts what one listing did with session files.
type Stats struct {
	Parsed    int `json:"parsed"`
	Cached    int `json:"cached"`
	Malformed int `json:"malformed"`
}
