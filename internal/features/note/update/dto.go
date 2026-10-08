package update

// Request is the attributes of the PATCH /api/notes/:id document. A JSON:API
// PATCH is partial: an attribute left out (nil) keeps its current value.
type Request struct {
	Title *string `json:"title" binding:"omitempty,notblank,max=255" label:"Title"`
	Body  *string `json:"body" binding:"omitempty,max=10000" label:"Content"`
}
