package me

import "time"

// MeResource is the caller, serialized as a JSON:API "users" resource. role
// is read live from the DB, so it is current even before the access token is
// refreshed.
type MeResource struct {
	ID        string    `json:"id" jsonapi:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
