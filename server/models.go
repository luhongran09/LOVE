package main

type User struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	SpaceID string `json:"space_id"`
}
type Photo struct {
	ID        string   `json:"id"`
	Filename  string   `json:"filename"`
	Title     string   `json:"title"`
	Note      string   `json:"note"`
	Place     string   `json:"place"`
	TakenAt   string   `json:"taken_at"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	OwnerID   string   `json:"owner_id"`
	CreatedAt string   `json:"created_at"`
}
