package domain

import "time"

// Announcement represents an upcoming primary placement or new issue announcement.
type Announcement struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Issuer       string    `json:"issuer"`
	Exchange     Exchange  `json:"exchange"`
	VolumeRUB    float64   `json:"volume_rub"`
	TargetCoupon string    `json:"target_coupon"`
	BookDate     time.Time `json:"book_date"`
	PlacedDate   time.Time `json:"placed_date"`
	DetailsURL   string    `json:"details_url"`
}
